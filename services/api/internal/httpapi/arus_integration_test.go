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

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/onboarding"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/platformsetting"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/qrisgate"
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

func TestARUSSSOCodeIsSingleUse(t *testing.T) {
	w := setup(t)
	if _, err := w.db.Exec(t.Context(), `UPDATE merchants SET legal_name = 'Legacy Toko', display_name = 'Legacy Toko' WHERE id = $1`, w.midA); err != nil {
		t.Fatal(err)
	}
	const token = "arus-provision-test-token-that-is-long-enough"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewRouter(Deps{Log: log, Auth: w.svc, Onboarding: onboarding.NewService(w.db, w.acct, onboarding.Config{Log: log}),
		ARUSProvisionToken: token, WebURL: "https://pay.test"}))
	t.Cleanup(server.Close)

	tenant := uuid.New()
	link := arusRequest(t, server, token, http.MethodPost, "/v1/integrations/arus/tenants",
		fmt.Sprintf(`{"tenant_id":%q,"business_name":"Legacy Toko","owner_name":"Pemilik Toko","owner_email":"owner@a.test","accepted_terms":true}`, tenant))
	if link.StatusCode != http.StatusOK {
		t.Fatalf("link: %d", link.StatusCode)
	}
	if r := arusRequest(t, server, token, http.MethodPost, "/v1/integrations/arus/tenants/"+uuid.NewString()+"/sso", ""); r.StatusCode != http.StatusNotFound {
		t.Fatalf("unlinked tenant sso: %d", r.StatusCode)
	}
	issued := decode(t, arusRequest(t, server, token, http.MethodPost, "/v1/integrations/arus/tenants/"+tenant.String()+"/sso", ""))
	url, _ := issued["url"].(string)
	code := strings.TrimPrefix(url, "https://pay.test/sso?code=")
	if code == "" || code == url {
		t.Fatalf("sso url: %v", issued)
	}
	exchange := func() *http.Response {
		resp, err := http.Post(server.URL+"/v1/auth/sso/exchange", "application/json", strings.NewReader(fmt.Sprintf(`{"code":%q}`, code)))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	first := exchange()
	if first.StatusCode != http.StatusOK || readCookie(first, sessionCookie) == "" {
		t.Fatalf("first exchange: %d", first.StatusCode)
	}
	if again := exchange(); again.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replayed code: %d", again.StatusCode)
	}
}

// Token key terpisah: token provisioning TIDAK boleh menerbitkan key, dan rute mati bila token key kosong.
func TestARUSKeyRouteNeedsItsOwnToken(t *testing.T) {
	const provision, keyTok = "arus-provision-test-token-that-is-long-enough", "arus-key-test-token-that-is-long-enough-xx"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	path := "/v1/integrations/arus/tenants/" + uuid.NewString() + "/api-key"
	srv := httptest.NewServer(NewRouter(Deps{Log: log, ARUSProvisionToken: provision, ARUSKeyToken: keyTok}))
	t.Cleanup(srv.Close)
	if r := arusRequest(t, srv, provision, http.MethodPost, path, ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("provision token must not issue keys: %d", r.StatusCode)
	}
	if r := arusRequest(t, srv, "salah", http.MethodPost, path, ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", r.StatusCode)
	}
	if r := arusRequest(t, srv, keyTok, http.MethodPost, path, ""); r.StatusCode != http.StatusBadRequest { // lolos auth; layanan belum dipasang
		t.Fatalf("key token passes auth: %d", r.StatusCode)
	}
	off := httptest.NewServer(NewRouter(Deps{Log: log, ARUSProvisionToken: provision}))
	t.Cleanup(off.Close)
	if r := arusRequest(t, off, keyTok, http.MethodPost, path, ""); r.StatusCode == http.StatusCreated || r.StatusCode == http.StatusBadRequest {
		t.Fatalf("route must be off without key token: %d", r.StatusCode)
	}
}

func TestARUSIssueKeyRequiresApprovedMerchantAndIsSingle(t *testing.T) {
	w := setup(t)
	if _, err := w.db.Exec(t.Context(), `UPDATE merchants SET legal_name = 'Legacy Toko', display_name = 'Legacy Toko' WHERE id = $1`, w.midA); err != nil {
		t.Fatal(err)
	}
	const provision, keyTok = "arus-provision-test-token-that-is-long-enough", "arus-key-test-token-that-is-long-enough-xx"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewRouter(Deps{Log: log, Onboarding: onboarding.NewService(w.db, w.acct, onboarding.Config{Log: log}),
		APIKeys: apikey.NewService(w.db, "sandbox"), ARUSProvisionToken: provision, ARUSKeyToken: keyTok}))
	t.Cleanup(server.Close)
	tenant := uuid.New()
	if r := arusRequest(t, server, keyTok, http.MethodPost, "/v1/integrations/arus/tenants/"+tenant.String()+"/api-key", ""); r.StatusCode != http.StatusNotFound {
		t.Fatalf("unlinked: %d", r.StatusCode)
	}
	link := arusRequest(t, server, provision, http.MethodPost, "/v1/integrations/arus/tenants",
		fmt.Sprintf(`{"tenant_id":%q,"business_name":"Legacy Toko","owner_name":"Pemilik Toko","owner_email":"owner@a.test","accepted_terms":true}`, tenant))
	if link.StatusCode != http.StatusOK {
		t.Fatalf("link: %d", link.StatusCode)
	}
	url := "/v1/integrations/arus/tenants/" + tenant.String() + "/api-key"
	if _, err := w.db.Exec(t.Context(), `UPDATE merchants SET onboarding_status = 'submitted' WHERE id = $1`, w.midA); err != nil {
		t.Fatal(err)
	}
	if r := arusRequest(t, server, keyTok, http.MethodPost, url, ""); r.StatusCode != http.StatusConflict {
		t.Fatalf("unapproved merchant: %d", r.StatusCode)
	}
	if _, err := w.db.Exec(t.Context(), `UPDATE merchants SET onboarding_status = 'approved' WHERE id = $1`, w.midA); err != nil {
		t.Fatal(err)
	}
	first := arusRequest(t, server, keyTok, http.MethodPost, url, "")
	issued := decode(t, first)
	secret, _ := issued["secret"].(string)
	scopes := fmt.Sprint(issued["scopes"])
	if first.StatusCode != http.StatusCreated || !strings.HasPrefix(secret, "epk_") || scopes != "[payment:create payment:read]" {
		t.Fatalf("issue: %d %v", first.StatusCode, issued)
	}
	if r := arusRequest(t, server, keyTok, http.MethodPost, url, ""); r.StatusCode != http.StatusConflict {
		t.Fatalf("second key must be refused: %d", r.StatusCode)
	}
}

func TestARUSListenerSetupReusesSecretAndNeedsKeyToken(t *testing.T) {
	w := setup(t)
	if _, err := w.db.Exec(t.Context(), `UPDATE merchants SET legal_name = 'Legacy Toko', display_name = 'Legacy Toko', mode = 'production' WHERE id = $1`, w.midA); err != nil {
		t.Fatal(err)
	}
	const provision, keyTok = "arus-provision-test-token-that-is-long-enough", "arus-key-test-token-that-is-long-enough-xx"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	qg := qrisgate.NewService(w.db, platformsetting.NewService(w.db), nil)
	qg.SetBox(testBox(t))
	server := httptest.NewServer(NewRouter(Deps{Log: log, Onboarding: onboarding.NewService(w.db, w.acct, onboarding.Config{Log: log}),
		QRISGate: qg, PublicAPIURL: "https://api.test", ARUSProvisionToken: provision, ARUSKeyToken: keyTok}))
	t.Cleanup(server.Close)
	tenant := uuid.New()
	path := "/v1/integrations/arus/tenants/" + tenant.String() + "/listener-setup"
	if r := arusRequest(t, server, provision, http.MethodPost, path, ""); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("provision token must not read the secret: %d", r.StatusCode)
	}
	if r := arusRequest(t, server, keyTok, http.MethodPost, path, ""); r.StatusCode != http.StatusNotFound {
		t.Fatalf("unlinked: %d", r.StatusCode)
	}
	if r := arusRequest(t, server, provision, http.MethodPost, "/v1/integrations/arus/tenants",
		fmt.Sprintf(`{"tenant_id":%q,"business_name":"Legacy Toko","owner_name":"Pemilik Toko","owner_email":"owner@a.test","accepted_terms":true}`, tenant)); r.StatusCode != http.StatusOK {
		t.Fatalf("link: %d", r.StatusCode)
	}
	first := arusRequest(t, server, keyTok, http.MethodPost, path, "")
	a := decode(t, first)
	l1, _ := a["link"].(string)
	if first.StatusCode != http.StatusOK || !strings.HasPrefix(l1, "erapaylistener://setup?") ||
		!strings.Contains(l1, "url=https%3A%2F%2Fapi.test%2Fwebhooks%2Fqrisgate%2F") || !strings.Contains(l1, "secret=qls_") {
		t.Fatalf("first: %d %v", first.StatusCode, a)
	}
	if b, _ := decode(t, arusRequest(t, server, keyTok, http.MethodPost, path, ""))["link"].(string); b != l1 {
		t.Fatalf("secret must be reused: %q vs %q", b, l1)
	}
}
