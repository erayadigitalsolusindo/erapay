package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/ledger"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/pricing"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// prodWorld is the standard test world plus a second router over the same database that runs as a production server.
func prodWorld(t *testing.T) (*world, *world) {
	t.Helper()
	w := setup(t)
	p := &world{svc: w.svc, db: w.db}
	p.srv = httptest.NewServer(NewRouter(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: fakePinger{}, Redis: fakePinger{},
		Auth: w.svc, Merchants: merchant.NewRepo(w.db), APIKeys: apikey.NewService(w.db, "production"),
	}))
	t.Cleanup(p.srv.Close)
	return w, p
}

func TestSandboxTwinAndKeysHTTP(t *testing.T) {
	w, p := prodWorld(t)
	owner := p.client(t, "owner@a.test")
	cashier := p.client(t, "cashier@a.test")
	ownerB := p.client(t, "owner@b.test")
	anon := &http.Client{}

	if r := p.do(t, cashier, "POST", "/v1/sandbox", `{}`, nil); r.StatusCode != 403 {
		t.Fatalf("cashier enabling sandbox: %d", r.StatusCode)
	}
	if r := p.do(t, owner, "POST", "/v1/sandbox/api-keys", `{"name":"x","scopes":["payment:read"]}`, nil); r.StatusCode != 404 {
		t.Fatalf("sandbox key before enabling: %d", r.StatusCode)
	}

	r := p.do(t, owner, "POST", "/v1/sandbox", `{}`, nil)
	twin := decode(t, r)
	if r.StatusCode != 201 || twin["merchant_id"] == "" || twin["merchant_id"] == w.midA {
		t.Fatalf("enable: %d %v", r.StatusCode, twin)
	}
	r = p.do(t, owner, "POST", "/v1/sandbox", `{}`, nil)
	if again := decode(t, r); r.StatusCode != 200 || again["merchant_id"] != twin["merchant_id"] {
		t.Fatalf("enable twice: %d %v", r.StatusCode, again)
	}

	// Sandbox key -> the twin, sandbox. Production key -> the real merchant, production.
	sbx := decode(t, p.do(t, owner, "POST", "/v1/sandbox/api-keys", `{"name":"sbx","scopes":["payment:create","payment:read"]}`, nil))
	prd := decode(t, p.do(t, owner, "POST", "/v1/api-keys", `{"name":"prd","scopes":["payment:create","payment:read"]}`, nil))
	sbxSecret, _ := sbx["secret"].(string)
	prdSecret, _ := prd["secret"].(string)
	if !strings.HasPrefix(sbxSecret, "epk_sbx_") || !strings.HasPrefix(prdSecret, "epk_prd_") {
		t.Fatalf("formats: %q %q", sbxSecret, prdSecret)
	}
	if self := decode(t, p.do(t, anon, "GET", "/v1/api-keys/self", "", bearer(sbxSecret))); self["merchant_id"] != twin["merchant_id"] || self["environment"] != "sandbox" {
		t.Fatalf("sandbox key self: %v", self)
	}
	if self := decode(t, p.do(t, anon, "GET", "/v1/api-keys/self", "", bearer(prdSecret))); self["merchant_id"] != w.midA || self["environment"] != "production" {
		t.Fatalf("production key self: %v", self)
	}

	// Each list shows only its own world's keys; the secrets never come back.
	prdList := decode(t, p.do(t, owner, "GET", "/v1/api-keys", "", nil))
	sbxList := decode(t, p.do(t, owner, "GET", "/v1/sandbox/api-keys", "", nil))
	if n := len(prdList["data"].([]any)); n != 1 {
		t.Fatalf("production keys listed: %d", n)
	}
	if n := len(sbxList["data"].([]any)); n != 1 {
		t.Fatalf("sandbox keys listed: %d", n)
	}

	// Another tenant cannot see or revoke this twin's keys.
	if r := p.do(t, ownerB, "GET", "/v1/sandbox/api-keys", "", nil); r.StatusCode != 404 {
		t.Fatalf("other tenant sees the sandbox: %d", r.StatusCode)
	}
	if r := p.do(t, ownerB, "POST", "/v1/sandbox", `{}`, nil); r.StatusCode != 201 {
		t.Fatalf("other tenant enabling its own sandbox: %d", r.StatusCode)
	}
	if r := p.do(t, ownerB, "POST", "/v1/sandbox/api-keys/"+sbx["id"].(string)+"/revoke", `{}`, nil); r.StatusCode != 404 {
		t.Fatalf("other tenant revoking: %d", r.StatusCode)
	}

	// Revoke through the sandbox route kills only that key.
	if r := p.do(t, owner, "POST", "/v1/sandbox/api-keys/"+sbx["id"].(string)+"/revoke", `{}`, nil); r.StatusCode != 204 {
		t.Fatalf("revoke: %d", r.StatusCode)
	}
	if r := p.do(t, anon, "GET", "/v1/api-keys/self", "", bearer(sbxSecret)); r.StatusCode != 401 {
		t.Fatalf("revoked sandbox key: %d", r.StatusCode)
	}
	if r := p.do(t, anon, "GET", "/v1/api-keys/self", "", bearer(prdSecret)); r.StatusCode != 200 {
		t.Fatalf("production key after sandbox revoke: %d", r.StatusCode)
	}

	// A sandbox-environment server never offers a twin: everything there is already sandbox.
	if r := w.do(t, w.client(t, "owner@a.test"), "POST", "/v1/sandbox", `{}`, nil); r.StatusCode != 409 {
		t.Fatalf("sandbox server enable: %d", r.StatusCode)
	}
}

// Deleting a merchant takes its sandbox twin along, unless the twin holds financial history.
func TestDeleteMerchantWithSandboxTwin(t *testing.T) {
	w := setup(t)
	repo := merchant.NewRepo(w.db)
	ctx := t.Context()
	count := func(id string) int {
		var n int
		_ = w.db.QueryRow(ctx, `SELECT count(*) FROM merchants WHERE id = $1`, id).Scan(&n)
		return n
	}

	clean, _ := testdb.SeedMerchant(t, w.db, "C")
	twin, _, err := repo.EnableSandbox(ctx, clean, "", "r")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.svc.DeleteMerchant(ctx, clean, "", "r"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if count(clean) != 0 || count(twin) != 0 {
		t.Fatal("merchant or its twin survived")
	}

	busy, _ := testdb.SeedMerchant(t, w.db, "D")
	btwin, _, err := repo.EnableSandbox(ctx, busy, "", "r")
	if err != nil {
		t.Fatal(err)
	}
	txn, err := ledger.PaymentSucceeded(btwin, "sbx-pay-1", pricing.Breakdown{Gross: 1000, MDRFee: 7, PlatformFee: 10, MerchantNet: 983, Currency: "IDR"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.NewService(w.db).Post(ctx, txn); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.DeleteMerchant(ctx, busy, "", "r"); !errors.Is(err, auth.ErrMerchantHasFinancial) {
		t.Fatalf("merchant whose twin has sandbox journals: %v", err)
	}
	if count(busy) != 1 || count(btwin) != 1 {
		t.Fatal("a refused delete removed something")
	}
}
