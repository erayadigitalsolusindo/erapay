package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// A notification only ever touches payments of its own world: /webhooks/{gw} the real merchants',
// /webhooks/sandbox/{gw} the sandbox merchants'. A cross-world reference is "not found", never applied.
func TestSandboxWebhookIsolatedFromProduction(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	live, liveOutlet := testdb.SeedMerchant(t, db, "Live")
	var twin, twinOutlet, plan string
	if err := db.QueryRow(ctx, `INSERT INTO merchants (legal_name, display_name, onboarding_status, is_sandbox, parent_id)
		VALUES ('Live sbx','Live sbx','approved',true,$1) RETURNING id::text`, live).Scan(&twin); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO merchant_outlets (merchant_id, name) VALUES ($1,'o') RETURNING id::text`, twin).Scan(&twinOutlet); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('std', 1, 'gw1', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	for _, m := range [][2]string{{live, "production"}, {twin, "sandbox"}} {
		if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from) VALUES ($1,$2, now() - interval '1 day')`, m[0], plan); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO merchant_gateway_accounts (merchant_id, gateway, environment, gateway_ref, activation_status)
			VALUES ($1,'gw1',$2,'MID-1','active')`, m[0], m[1]); err != nil {
			t.Fatal(err)
		}
	}

	prod, sbx := gateway.NewRegistry(), gateway.NewRegistry()
	prod.Register(verifyingGW{newTestGW()})
	sbx.Register(verifyingGW{newTestGW()})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	flow := payment.NewOrchestrator(payment.NewService(db), db, idempotency.NewStore(db), prod, "production", log)
	flow.SetSandbox(sbx)
	srv := httptest.NewServer(NewRouter(Deps{Log: log, DB: fakePinger{}, Redis: fakePinger{}, Gateways: prod, SandboxGateways: sbx, PaymentFlow: flow}))
	defer srv.Close()

	newPayment := func(mid, outlet, key string) string {
		h, _ := idempotency.HashJSON([]byte(`{"k":"`+key+`"}`), "POST", "/v1/payments")
		out, err := flow.Create(ctx, payment.CreateRequest{MerchantID: mid, OutletID: outlet, MerchantReference: key,
			Amount: 10000, Currency: "IDR", PaymentMethod: "QRIS", IdempotencyKey: key, RequestHash: h})
		if err != nil || out.Payment.Status != payment.Pending {
			t.Fatalf("setup: %+v %v", out, err)
		}
		return out.Payment.ID
	}
	send := func(path, sig, ref, id string) int {
		req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(`{}`))
		req.Header.Set("X-Test-Sig", sig)
		req.Header.Set("X-Ref", ref)
		req.Header.Set("X-Id", id)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	status := func(id string) string {
		var s string
		if err := db.QueryRow(ctx, `SELECT status FROM payments WHERE id=$1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	journals := func(livemode bool) int {
		var n int
		_ = db.QueryRow(ctx, `SELECT count(DISTINCT le.txn_id) FROM ledger_entries le JOIN ledger_accounts a ON a.id = le.account_id WHERE a.livemode = $1`, livemode).Scan(&n)
		return n
	}

	sp := newPayment(twin, twinOutlet, "key-sbx-wh-0001")
	lp := newPayment(live, liveOutlet, "key-live-wh-0001")

	// Cross-world references are acknowledged (a retry cannot fix them) but change nothing.
	if c := send("/webhooks/gw1", "valid", sp, "a1"); c != 200 || status(sp) != "pending" {
		t.Fatalf("sandbox payment via the real URL: %d status=%s", c, status(sp))
	}
	if c := send("/webhooks/sandbox/gw1", "valid", lp, "a2"); c != 200 || status(lp) != "pending" {
		t.Fatalf("real payment via the sandbox URL: %d status=%s", c, status(lp))
	}
	if journals(true)+journals(false) != 0 {
		t.Fatal("a cross-world notification posted a journal")
	}

	// Own world: signature is checked, then applied once, in the sandbox books only.
	if c := send("/webhooks/sandbox/gw1", "forged", sp, "b1"); c != 400 || status(sp) != "pending" {
		t.Fatalf("forged sandbox notification: %d status=%s", c, status(sp))
	}
	if c := send("/webhooks/sandbox/gw1", "valid", sp, "b2"); c != 200 || status(sp) != "succeeded" {
		t.Fatalf("sandbox notification: %d status=%s", c, status(sp))
	}
	if journals(false) != 1 || journals(true) != 0 {
		t.Fatalf("journals sandbox=%d live=%d, want 1/0", journals(false), journals(true))
	}
	if c := send("/webhooks/gw1", "valid", lp, "b3"); c != 200 || status(lp) != "succeeded" || journals(true) != 1 {
		t.Fatalf("real notification: %d status=%s live journals=%d", c, status(lp), journals(true))
	}

	// 20 concurrent retries of one sandbox notification: still one journal.
	sp2 := newPayment(twin, twinOutlet, "key-sbx-wh-0002")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c := send("/webhooks/sandbox/gw1", "valid", sp2, "c1"); c != 200 {
				t.Errorf("concurrent: %d", c)
			}
		}()
	}
	wg.Wait()
	if status(sp2) != "succeeded" || journals(false) != 2 || journals(true) != 1 {
		t.Fatalf("after concurrency: status=%s sandbox=%d live=%d", status(sp2), journals(false), journals(true))
	}
}
