package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/outbound"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payout"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/refund"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/settlement"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

const itPW = "correct horse battery"

type world struct {
	srv              *httptest.Server
	midA, midB       string
	outletA, outletB string
	svc              *auth.Service
	db               *pgxpool.Pool
	gw               *testGW
}

func setup(t *testing.T) *world {
	t.Helper()
	db := testdb.New(t)
	svc := auth.NewService(db, auth.DefaultConfig())
	w := &world{svc: svc, db: db}
	w.midA, w.outletA = testdb.SeedMerchant(t, db, "Merchant A")
	w.midB, w.outletB = testdb.SeedMerchant(t, db, "Merchant B")
	mk := func(email, role, mid string) {
		uid, err := svc.CreateUser(t.Context(), email, itPW)
		if err != nil {
			t.Fatal(err)
		}
		testdb.AddMember(t, db, mid, uid, role)
	}
	mk("owner@a.test", "merchant_owner", w.midA)
	mk("cashier@a.test", "cashier", w.midA)
	mk("integrator@a.test", "integrator", w.midA)
	mk("owner@b.test", "merchant_owner", w.midB)
	root, _ := svc.CreateUser(t.Context(), "root@erapay.test", itPW)
	testdb.AddPlatformRole(t, db, root, "super_admin")
	fin, _ := svc.CreateUser(t.Context(), "finance@erapay.test", itPW)
	testdb.AddPlatformRole(t, db, fin, "finance")

	seedPayments(t, db, w.midA, w.midB)
	w.gw = newTestGW()
	reg := gateway.NewRegistry()
	reg.Register(w.gw)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	paySvc := payment.NewService(db)
	w.srv = httptest.NewServer(NewRouter(Deps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), DB: fakePinger{}, Redis: fakePinger{},
		Auth: svc, Merchants: merchant.NewRepo(db), APIKeys: apikey.NewService(db, "sandbox"), Payments: paySvc,
		PaymentFlow: payment.NewOrchestrator(paySvc, db, idempotency.NewStore(db), reg, "sandbox", log),
		Outbound:    outbound.NewService(db, testBox(t), log),
		Settlement:  settlement.NewService(db),
		Refunds:     refund.NewService(db),
		Payouts:     payout.NewService(db, testBox(t), payout.Config{DryRun: true, MinAmount: 10_000, MaxAmount: 1_000_000, DailyLimit: 5_000_000}),
	}))
	t.Cleanup(w.srv.Close)
	return w
}

func (w *world) client(t *testing.T, email string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp := w.do(t, c, "POST", "/v1/auth/login", `{"email":"`+email+`","password":"`+itPW+`"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("login %s: %d", email, resp.StatusCode)
	}
	return c
}

func (w *world) do(t *testing.T, c *http.Client, method, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, w.srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" || method != "GET" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decode(t *testing.T, r *http.Response) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

// Invariant #7: tenant comes only from the authenticated session.
func TestTenantIsolation(t *testing.T) {
	w := setup(t)
	a := w.client(t, "owner@a.test")
	b := w.client(t, "owner@b.test")

	// Each owner sees only its own profile.
	if got := decode(t, w.do(t, a, "GET", "/v1/merchant/profile", "", nil))["id"]; got != w.midA {
		t.Fatalf("A saw %v", got)
	}
	if got := decode(t, w.do(t, b, "GET", "/v1/merchant/profile", "", nil))["id"]; got != w.midB {
		t.Fatalf("B saw %v", got)
	}

	// Spoofing attempts via header / query / body are ignored.
	spoofs := []struct {
		path string
		hdr  map[string]string
	}{
		{"/v1/merchant/profile?merchant_id=" + w.midB, nil},
		{"/v1/merchant/profile", map[string]string{"X-Merchant-ID": w.midB, "X-Forwarded-Merchant": w.midB}},
		{"/v1/merchant/outlets?merchant_id=" + w.midB, map[string]string{"X-Merchant-Id": w.midB}},
	}
	for _, s := range spoofs {
		resp := w.do(t, a, "GET", s.path, "", s.hdr)
		body := decode(t, resp)
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d", s.path, resp.StatusCode)
		}
		raw, _ := json.Marshal(body)
		if strings.Contains(string(raw), w.midB) || strings.Contains(string(raw), w.outletB) {
			t.Fatalf("tenant leak via %s: %s", s.path, raw)
		}
	}

	// Outlet lists are disjoint.
	oa, _ := json.Marshal(decode(t, w.do(t, a, "GET", "/v1/merchant/outlets", "", nil)))
	if !strings.Contains(string(oa), w.outletA) || strings.Contains(string(oa), w.outletB) {
		t.Fatalf("A outlets: %s", oa)
	}
	ob, _ := json.Marshal(decode(t, w.do(t, b, "GET", "/v1/merchant/outlets", "", nil)))
	if !strings.Contains(string(ob), w.outletB) || strings.Contains(string(ob), w.outletA) {
		t.Fatalf("B outlets: %s", ob)
	}

	// A user of A cannot log in into B by naming it.
	c := &http.Client{}
	resp := w.do(t, c, "POST", "/v1/auth/login",
		`{"email":"owner@a.test","password":"`+itPW+`","merchant_id":"`+w.midB+`"}`, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("cross-tenant login selector: %d", resp.StatusCode)
	}
}

func TestAuthRequiredAndRBAC(t *testing.T) {
	w := setup(t)
	anon := &http.Client{}
	if r := w.do(t, anon, "GET", "/v1/merchant/profile", "", nil); r.StatusCode != 401 {
		t.Fatalf("anon: %d", r.StatusCode)
	}
	if r := w.do(t, anon, "GET", "/v1/merchant/profile", "", map[string]string{"Cookie": sessionCookie + "=forged"}); r.StatusCode != 401 {
		t.Fatalf("forged: %d", r.StatusCode)
	}

	// cashier: has outlet:read but not merchant:read.
	cash := w.client(t, "cashier@a.test")
	if r := w.do(t, cash, "GET", "/v1/merchant/profile", "", nil); r.StatusCode != 403 {
		t.Fatalf("cashier profile: %d", r.StatusCode)
	}
	if r := w.do(t, cash, "GET", "/v1/merchant/outlets", "", nil); r.StatusCode != 200 {
		t.Fatalf("cashier outlets: %d", r.StatusCode)
	}
	// integrator: neither.
	integ := w.client(t, "integrator@a.test")
	if r := w.do(t, integ, "GET", "/v1/merchant/outlets", "", nil); r.StatusCode != 403 {
		t.Fatalf("integrator outlets: %d", r.StatusCode)
	}

	// platform session has no tenant: tenant routes are forbidden.
	root := w.client(t, "root@erapay.test")
	if r := w.do(t, root, "GET", "/v1/merchant/profile", "", nil); r.StatusCode != 403 {
		t.Fatalf("platform on tenant route: %d", r.StatusCode)
	}
}

func TestLoginLogoutHTTP(t *testing.T) {
	w := setup(t)
	c := &http.Client{}
	if r := w.do(t, c, "POST", "/v1/auth/login", `{"email":"owner@a.test","password":"nope-nope-nope"}`, nil); r.StatusCode != 401 {
		t.Fatalf("bad pw: %d", r.StatusCode)
	}
	if r := w.do(t, c, "POST", "/v1/auth/login", `{}`, nil); r.StatusCode != 400 {
		t.Fatalf("empty: %d", r.StatusCode)
	}
	// Form content-type is rejected (CSRF defense in depth).
	req, _ := http.NewRequest("POST", w.srv.URL+"/v1/auth/login", strings.NewReader("email=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := c.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 415 {
		t.Fatalf("form post: %d", resp.StatusCode)
	}

	jar, _ := cookiejar.New(nil)
	c = &http.Client{Jar: jar}
	resp = w.do(t, c, "POST", "/v1/auth/login", `{"email":"owner@a.test","password":"`+itPW+`"}`, nil)
	var ck *http.Cookie
	for _, x := range resp.Cookies() {
		if x.Name == sessionCookie {
			ck = x
		}
	}
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode {
		t.Fatalf("bad session cookie: %+v", ck)
	}
	if r := w.do(t, c, "GET", "/v1/auth/me", "", nil); r.StatusCode != 200 {
		t.Fatalf("me: %d", r.StatusCode)
	}
	if r := w.do(t, c, "POST", "/v1/auth/logout", "{}", nil); r.StatusCode != 204 {
		t.Fatalf("logout: %d", r.StatusCode)
	}
	// Replaying the old cookie after logout is rejected server-side.
	if r := w.do(t, &http.Client{}, "GET", "/v1/auth/me", "", map[string]string{"Cookie": sessionCookie + "=" + ck.Value}); r.StatusCode != 401 {
		t.Fatalf("revoked cookie accepted: %d", r.StatusCode)
	}
}

func TestLoginRateLimitHTTP(t *testing.T) {
	w := setup(t)
	c := &http.Client{}
	var last int
	for i := 0; i < 6; i++ {
		last = w.do(t, c, "POST", "/v1/auth/login", `{"email":"owner@a.test","password":"wrong-wrong-wrong"}`, nil).StatusCode
	}
	if last != 429 {
		t.Fatalf("want 429 after repeated failures, got %d", last)
	}
	if r := w.do(t, c, "POST", "/v1/auth/login", `{"email":"owner@a.test","password":"`+itPW+`"}`, nil); r.StatusCode != 429 {
		t.Fatalf("lockout bypassed: %d", r.StatusCode)
	}
}
