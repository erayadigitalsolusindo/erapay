package httpapi

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/onboarding"
)

func arusRequest(t *testing.T, server *httptest.Server, token, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestARUSProvisioningLinksExistingAndCreatesIdempotently(t *testing.T) {
	w := setup(t)
	if _, err := w.db.Exec(t.Context(), `UPDATE merchants SET legal_name = 'Legacy Toko', display_name = 'Legacy Toko' WHERE id = $1`, w.midA); err != nil {
		t.Fatal(err)
	}
	const token = "arus-provision-test-token-that-is-long-enough"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := onboarding.NewService(w.db, w.acct, onboarding.Config{Log: log})
	server := httptest.NewServer(NewRouter(Deps{Log: log, Onboarding: service, ARUSProvisionToken: token}))
	t.Cleanup(server.Close)

	legacyTenant := uuid.New()
	legacyBody := fmt.Sprintf(`{"tenant_id":%q,"business_name":"Legacy Toko","owner_name":"Pemilik Toko","owner_email":"owner@a.test","accepted_terms":true}`, legacyTenant.String())
	first := arusRequest(t, server, token, http.MethodPost, "/v1/integrations/arus/tenants", legacyBody)
	linked := decode(t, first)
	if first.StatusCode != http.StatusOK || linked["merchant_id"] != w.midA || linked["created"] == true || linked["owner_role"] != "merchant_owner" {
		t.Fatalf("legacy link: %d %v", first.StatusCode, linked)
	}
	second := arusRequest(t, server, token, http.MethodPost, "/v1/integrations/arus/tenants", legacyBody)
	linkedAgain := decode(t, second)
	if second.StatusCode != http.StatusOK || linkedAgain["merchant_id"] != w.midA {
		t.Fatalf("legacy retry: %d %v", second.StatusCode, linkedAgain)
	}

	newTenant := uuid.New()
	newBody := fmt.Sprintf(`{"tenant_id":%q,"business_name":"Toko Baru","owner_name":"Pemilik Baru","owner_email":"pemilik-baru@toko.test","owner_phone":"+6281234567890","accepted_terms":true}`, newTenant.String())
	created := arusRequest(t, server, token, http.MethodPost, "/v1/integrations/arus/tenants", newBody)
	createdResult := decode(t, created)
	if created.StatusCode != http.StatusOK || createdResult["created"] != true || createdResult["owner_role"] != "merchant_owner" {
		t.Fatalf("create: %d %v", created.StatusCode, createdResult)
	}
	merchantID := createdResult["merchant_id"]
	retry := arusRequest(t, server, token, http.MethodPost, "/v1/integrations/arus/tenants", newBody)
	retriedResult := decode(t, retry)
	if retry.StatusCode != http.StatusOK || retriedResult["merchant_id"] != merchantID || retriedResult["created"] == true {
		t.Fatalf("create retry: %d %v", retry.StatusCode, retriedResult)
	}
	if token := mailLink(t, w, "pemilik-baru@toko.test", "/reset-password"); token == "" {
		t.Fatal("owner setup email was not sent")
	}
}
