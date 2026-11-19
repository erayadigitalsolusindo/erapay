package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/apikey"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway/fake"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/merchant"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

// fakeWorld is a production server with the fake sandbox gateway on: merchant A (with a DOKU plan), its owner,
// and merchant B, over the standard test database.
func fakeWorld(t *testing.T) (*world, *world) {
	t.Helper()
	w := setup(t)
	ctx := context.Background()
	var plan string
	if err := w.db.QueryRow(ctx, `INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('doku-a', 1, 'doku', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from) VALUES ($1,$2, now() - interval '1 day')`, w.midA, plan); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	prod, sbx := gateway.NewRegistry(), gateway.NewRegistry()
	sbx.Register(fake.New())
	paySvc := payment.NewService(w.db)
	flow := payment.NewOrchestrator(paySvc, w.db, idempotency.NewStore(w.db), prod, "production", log)
	flow.SetSandbox(sbx)
	repo := merchant.NewRepo(w.db)
	repo.SetFakeGateway(true)
	p := &world{svc: w.svc, db: w.db}
	p.srv = httptest.NewServer(NewRouter(Deps{Log: log, DB: fakePinger{}, Redis: fakePinger{}, Auth: w.svc, Merchants: repo,
		APIKeys: apikey.NewService(w.db, "production"), Payments: paySvc, PaymentFlow: flow, Gateways: prod, SandboxGateways: sbx}))
	t.Cleanup(p.srv.Close)
	return w, p
}

func TestSandboxSimulatorEndToEnd(t *testing.T) {
	w, p := fakeWorld(t)
	owner := p.client(t, "owner@a.test")
	anon := &http.Client{}
	ctx := context.Background()

	if r := p.do(t, owner, "POST", "/v1/sandbox", `{}`, nil); r.StatusCode != 201 {
		t.Fatalf("enable: %d", r.StatusCode)
	}
	sbx := decode(t, p.do(t, owner, "POST", "/v1/sandbox/api-keys", `{"name":"sbx","scopes":["payment:create","payment:read"]}`, nil))["secret"].(string)
	prd := decode(t, p.do(t, owner, "POST", "/v1/api-keys", `{"name":"prd","scopes":["payment:create","payment:read"]}`, nil))["secret"].(string)

	create := func(key, ref string) map[string]any {
		r := p.do(t, anon, "POST", "/v1/payments", `{"merchant_reference":"`+ref+`","amount":10000,"currency":"IDR","payment_method":"QRIS"}`,
			map[string]string{"Authorization": "Bearer " + sbx, "Idempotency-Key": key})
		pay := decode(t, r)
		if r.StatusCode != 201 || pay["status"] != "pending" || pay["gateway"] != "fake" {
			t.Fatalf("create: %d %v", r.StatusCode, pay)
		}
		return pay
	}
	simulate := func(secret, id, outcome string) (int, map[string]any) {
		r := p.do(t, anon, "POST", "/v1/sandbox/payments/"+id+"/simulate", `{"outcome":"`+outcome+`"}`, bearer(secret))
		return r.StatusCode, decode(t, r)
	}
	status := func(id string) string {
		var s string
		_ = w.db.QueryRow(ctx, `SELECT status FROM payments WHERE id=$1`, id).Scan(&s)
		return s
	}
	journals := func(live bool) int {
		var n int
		_ = w.db.QueryRow(ctx, `SELECT count(DISTINCT le.txn_id) FROM ledger_entries le JOIN ledger_accounts a ON a.id = le.account_id WHERE a.livemode = $1`, live).Scan(&n)
		return n
	}

	// A sandbox payment gets a QR from the fake gateway and fees priced like the merchant's real plan.
	pay := create("idem-sim-0001", "ORD-1")
	if fee, _ := pay["fee"].(map[string]any); fee["mdr_fee"] != float64(70) || fee["platform_fee"] != float64(100) {
		t.Fatalf("fee: %v", pay["fee"])
	}
	id := pay["id"].(string)

	// Only sandbox keys may simulate; bad outcomes and other tenants' payments are refused.
	if c, _ := simulate(prd, id, "paid"); c != 403 && c != 404 {
		t.Fatalf("production key simulating: %d", c)
	}
	if c, _ := simulate(sbx, id, "refunded"); c != 400 {
		t.Fatalf("unknown outcome: %d", c)
	}
	if c, _ := simulate(sbx, "00000000-0000-4000-8000-00000000abcd", "paid"); c != 404 {
		t.Fatalf("unknown payment: %d", c)
	}
	if status(id) != "pending" || journals(false)+journals(true) != 0 {
		t.Fatal("a refused simulation changed something")
	}

	// Paid: succeeded, booked once in the sandbox world only; repeating (even 15 at once) changes nothing.
	if c, out := simulate(sbx, id, "paid"); c != 200 || out["applied"] != true || out["payment"].(map[string]any)["status"] != "succeeded" {
		t.Fatalf("paid: %d %v", c, out)
	}
	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c, _ := simulate(sbx, id, "paid"); c != 200 {
				t.Errorf("repeat: %d", c)
			}
		}()
	}
	wg.Wait()
	if status(id) != "succeeded" || journals(false) != 1 || journals(true) != 0 {
		t.Fatalf("after repeats: status=%s sandbox journals=%d live=%d", status(id), journals(false), journals(true))
	}
	// A final payment never flips: a late "failed" is reported as not applied.
	if c, out := simulate(sbx, id, "failed"); c != 200 || out["applied"] == true || status(id) != "succeeded" {
		t.Fatalf("late failed: %d %v status=%s", c, out, status(id))
	}

	// The other outcomes.
	for outcome, want := range map[string]string{"expired": "expired", "failed": "failed"} {
		pp := create("idem-sim-"+outcome, "ORD-"+outcome)
		if c, out := simulate(sbx, pp["id"].(string), outcome); c != 200 || out["payment"].(map[string]any)["status"] != want {
			t.Fatalf("%s: %d %v", outcome, c, out)
		}
	}

	// The simulator is another tenant's blind spot: merchant B (no sandbox) has no key that reaches this payment.
	ownerB := p.client(t, "owner@b.test")
	if r := p.do(t, ownerB, "POST", "/v1/sandbox", `{}`, nil); r.StatusCode != 201 {
		t.Fatalf("B enable: %d", r.StatusCode)
	}
	bKey := decode(t, p.do(t, ownerB, "POST", "/v1/sandbox/api-keys", `{"name":"b","scopes":["payment:create"]}`, nil))["secret"].(string)
	if c, _ := simulate(bKey, id, "paid"); c != 404 {
		t.Fatalf("another tenant's sandbox key: %d", c)
	}
}

// The dashboard session can switch to the sandbox twin and back. The twin has no members: the session takes its
// permissions from the user's membership in the real merchant, so removing that membership ends the session.
func TestSwitchSessionToSandbox(t *testing.T) {
	w, p := fakeWorld(t)
	owner := p.client(t, "owner@a.test")
	anon := &http.Client{}
	ctx := context.Background()
	profileID := func(c *http.Client) any { return decode(t, p.do(t, c, "GET", "/v1/merchant/profile", "", nil))["id"] }
	switchTo := func(c *http.Client, mode string) int {
		return p.do(t, c, "POST", "/v1/sandbox/switch", `{"mode":"`+mode+`"}`, nil).StatusCode
	}

	// Before sandbox exists there is nothing to switch to.
	if st := decode(t, p.do(t, owner, "GET", "/v1/sandbox", "", nil)); st["enabled"] != false || st["active"] != false || st["available"] != true {
		t.Fatalf("status before: %v", st)
	}
	if c := switchTo(owner, "sandbox"); c != 409 {
		t.Fatalf("switch without sandbox: %d", c)
	}
	if c := switchTo(owner, "elsewhere"); c != 400 {
		t.Fatalf("bad mode: %d", c)
	}
	twin := decode(t, p.do(t, owner, "POST", "/v1/sandbox", `{}`, nil))["merchant_id"]
	if st := decode(t, p.do(t, owner, "GET", "/v1/sandbox", "", nil)); st["enabled"] != true || st["active"] != false || st["simulator"] != true {
		t.Fatalf("status after enable: %v", st)
	}

	// A sandbox payment made with a sandbox key shows up in sandbox mode only.
	sbxKey := decode(t, p.do(t, owner, "POST", "/v1/sandbox/api-keys", `{"name":"k","scopes":["payment:create","payment:read"]}`, nil))["secret"].(string)
	pay := decode(t, p.do(t, anon, "POST", "/v1/payments", `{"merchant_reference":"ORD-SW","amount":10000,"currency":"IDR","payment_method":"QRIS"}`,
		map[string]string{"Authorization": "Bearer " + sbxKey, "Idempotency-Key": "idem-switch-01"}))["id"].(string)
	ids := func(c *http.Client) map[string]bool {
		out := map[string]bool{}
		for _, row := range decode(t, p.do(t, c, "GET", "/v1/payments", "", nil))["data"].([]any) {
			out[row.(map[string]any)["id"].(string)] = true
		}
		return out
	}
	if ids(owner)[pay] {
		t.Fatal("live mode lists a sandbox payment")
	}

	if c := switchTo(owner, "sandbox"); c != 200 {
		t.Fatalf("switch to sandbox: %d", c)
	}
	if profileID(owner) != twin {
		t.Fatalf("profile in sandbox mode: %v, want twin %v", profileID(owner), twin)
	}
	if st := decode(t, p.do(t, owner, "GET", "/v1/sandbox", "", nil)); st["active"] != true {
		t.Fatalf("status in sandbox mode: %v", st)
	}
	if !ids(owner)[pay] {
		t.Fatal("sandbox mode does not list the sandbox payment")
	}
	// Same owner permissions on the twin (api keys are issued for the twin, in the sandbox environment).
	k := decode(t, p.do(t, owner, "POST", "/v1/api-keys", `{"name":"dash","scopes":["payment:read"]}`, nil))
	if s, _ := k["secret"].(string); len(s) < 8 || s[:8] != "epk_sbx_" {
		t.Fatalf("key created in sandbox mode: %v", k)
	}
	if c := switchTo(owner, "sandbox"); c != 200 { // idempotent
		t.Fatalf("switch again: %d", c)
	}

	// Back to live: the real merchant again, the sandbox payment gone from view.
	if c := switchTo(owner, "live"); c != 200 || profileID(owner) != w.midA {
		t.Fatalf("switch to live: %d profile=%v", c, profileID(owner))
	}
	if ids(owner)[pay] {
		t.Fatal("live mode lists a sandbox payment after switching back")
	}

	// Another tenant's session is unaffected, and cannot reach this twin by any switch.
	ownerB := p.client(t, "owner@b.test")
	if profileID(ownerB) != w.midB || switchTo(ownerB, "sandbox") != 409 || profileID(ownerB) != w.midB {
		t.Fatal("tenant B was affected by tenant A's switch")
	}

	// Membership in the real merchant is what keeps a sandbox session alive.
	_ = switchTo(owner, "sandbox")
	if _, err := w.db.Exec(ctx, `DELETE FROM merchant_users mu USING users u WHERE u.id = mu.user_id AND u.email = 'owner@a.test'`); err != nil {
		t.Fatal(err)
	}
	if r := p.do(t, owner, "GET", "/v1/merchant/profile", "", nil); r.StatusCode != 401 {
		t.Fatalf("sandbox session after losing membership: %d", r.StatusCode)
	}

	// Platform and impersonation sessions cannot switch.
	if _, err := p.svc.SwitchSandbox(ctx, &auth.Principal{MerchantID: w.midA, ImpersonatorID: "x"}, true, "r"); !errors.Is(err, auth.ErrSwitchNotAllowed) {
		t.Fatalf("impersonation switch: %v", err)
	}
	if _, err := p.svc.SwitchSandbox(ctx, &auth.Principal{}, true, "r"); !errors.Is(err, auth.ErrSwitchNotAllowed) {
		t.Fatalf("platform switch: %v", err)
	}
}
