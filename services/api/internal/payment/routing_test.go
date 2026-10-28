package payment

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
)

// platformGW is a TEST-ONLY gateway contracted once for the whole platform (like IDS): it needs no
// merchant gateway account.
type platformGW struct{ *fakeGW }

func (platformGW) Code() string { return "ids" }
func (platformGW) Capabilities() gateway.Capabilities {
	return gateway.Capabilities{PaymentMethods: []string{"QRIS"}, Inquiry: true, PlatformAccount: true}
}

type fixedRouting struct {
	gw  string
	err error
}

func (f fixedRouting) Preferred(context.Context) (string, error) { return f.gw, f.err }

// switchable lets one test change the platform's choice between calls.
type switchable struct{ gw atomic.Value }

func (s *switchable) Preferred(context.Context) (string, error) {
	v, _ := s.gw.Load().(string)
	return v, nil
}

type renv struct {
	oenv
	ids      *fakeGW
	idsAcct  atomic.Value // AccountRef the ids adapter received
	register func()
}

// rsetup is osetup plus an IDS-like platform gateway with its own pricing plan. The IDS adapter is
// NOT registered until register() is called, so tests can cover "chosen but not applied".
func rsetup(t *testing.T) *renv {
	t.Helper()
	e := osetup(t)
	ctx := context.Background()
	var plan string
	if err := e.db.QueryRow(ctx, `
		INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('std-ids', 1, 'ids', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
		VALUES ($1,$2, now() - interval '1 day')`, e.mid, plan); err != nil {
		t.Fatal(err)
	}
	r := &renv{oenv: e, ids: newFake()}
	r.ids.create = func(q gateway.CreateRequest) (gateway.CreateResult, error) {
		r.idsAcct.Store(q.AccountRef)
		return gateway.CreateResult{GatewayReference: "IDS-" + q.Reference, QRPayload: "QR-" + q.Reference, ResponseCode: "201"}, nil
	}
	reg := gateway.NewRegistry()
	reg.Register(e.gw)
	r.register = func() { reg.Register(platformGW{r.ids}) }
	r.orch = NewOrchestrator(e.svc, e.db, idempotency.NewStore(e.db), reg, "sandbox", slog.New(slog.NewTextHandler(io.Discard, nil)))
	return r
}

func TestPlatformChoiceRoutesNewPaymentsToThatGateway(t *testing.T) {
	r := rsetup(t)
	r.register()
	ctx := context.Background()

	// No choice yet: the merchant's own account (gw1) decides, exactly as before.
	out, err := r.orch.Create(ctx, r.req("key-route-0001", 10000))
	if err != nil || out.Payment.Gateway != "gw1" {
		t.Fatalf("no choice: %+v %v", out.Payment, err)
	}

	// Choose the platform gateway: no merchant account row is needed, and no account ref is sent.
	r.orch.SetRouting(fixedRouting{gw: "ids"})
	out, err = r.orch.Create(ctx, r.req("key-route-0002", 20000))
	if err != nil || out.Payment.Gateway != "ids" || out.Payment.Status != Pending {
		t.Fatalf("ids: %+v %v", out.Payment, err)
	}
	if ref, _ := r.idsAcct.Load().(string); ref != "" {
		t.Fatalf("ids adapter got account ref %q, want none", ref)
	}
	if r.gw.createCalls.Load() != 1 || r.ids.createCalls.Load() != 1 {
		t.Fatalf("calls gw1=%d ids=%d", r.gw.createCalls.Load(), r.ids.createCalls.Load())
	}

	// Choose the merchant-account gateway again: it goes through the merchant's own ref.
	r.orch.SetRouting(fixedRouting{gw: "gw1"})
	out, err = r.orch.Create(ctx, r.req("key-route-0003", 30000))
	if err != nil || out.Payment.Gateway != "gw1" {
		t.Fatalf("gw1: %+v %v", out.Payment, err)
	}
}

// Switching never touches a payment that already exists: a retried request with the same
// Idempotency-Key returns the original payment and never calls a gateway again.
func TestSwitchingRouteKeepsIdempotentReplaysOnTheOriginalPayment(t *testing.T) {
	r := rsetup(t)
	r.register()
	ctx := context.Background()
	sw := &switchable{}
	sw.gw.Store("gw1")
	r.orch.SetRouting(sw)

	first, err := r.orch.Create(ctx, r.req("key-switch-001", 10000))
	if err != nil || first.Payment.Gateway != "gw1" {
		t.Fatalf("%+v %v", first.Payment, err)
	}
	sw.gw.Store("ids")
	again, err := r.orch.Create(ctx, r.req("key-switch-001", 10000))
	if err != nil || !again.Replayed || again.Payment.ID != first.Payment.ID || again.Payment.Gateway != "gw1" {
		t.Fatalf("replay after switch: %+v %v", again, err)
	}
	if r.ids.createCalls.Load() != 0 || r.gw.createCalls.Load() != 1 {
		t.Fatalf("calls gw1=%d ids=%d", r.gw.createCalls.Load(), r.ids.createCalls.Load())
	}
	next, err := r.orch.Create(ctx, r.req("key-switch-002", 10000))
	if err != nil || next.Payment.Gateway != "ids" {
		t.Fatalf("new key after switch: %+v %v", next.Payment, err)
	}
}

// A choice that cannot serve this merchant right now falls back to the merchant's own account BEFORE
// any gateway call, so payments keep working and nothing can be charged twice.
func TestUnavailableChoiceFallsBackToTheMerchantsOwnAccount(t *testing.T) {
	ctx := context.Background()
	t.Run("chosen gateway not registered", func(t *testing.T) {
		r := rsetup(t) // ids never registered
		r.orch.SetRouting(fixedRouting{gw: "ids"})
		out, err := r.orch.Create(ctx, r.req("key-fallback-01", 10000))
		if err != nil || out.Payment.Gateway != "gw1" {
			t.Fatalf("%+v %v", out.Payment, err)
		}
		if r.ids.createCalls.Load() != 0 {
			t.Fatal("unregistered gateway was called")
		}
	})
	t.Run("choice unreadable", func(t *testing.T) {
		r := rsetup(t)
		r.register()
		r.orch.SetRouting(fixedRouting{err: errors.New("db down")})
		out, err := r.orch.Create(ctx, r.req("key-fallback-02", 10000))
		if err != nil || out.Payment.Gateway != "gw1" {
			t.Fatalf("%+v %v", out.Payment, err)
		}
	})
	t.Run("account gateway chosen but merchant account suspended", func(t *testing.T) {
		r := rsetup(t)
		r.register()
		if _, err := r.db.Exec(ctx, `UPDATE merchant_gateway_accounts SET activation_status = 'suspended'`); err != nil {
			t.Fatal(err)
		}
		r.orch.SetRouting(fixedRouting{gw: "gw1"})
		// Nothing else can serve them either: refused, not silently sent somewhere unexpected.
		if _, err := r.orch.Create(ctx, r.req("key-fallback-03", 10000)); !errors.Is(err, ErrMethodUnavailable) {
			t.Fatalf("want ErrMethodUnavailable, got %v", err)
		}
		// With the platform gateway chosen instead, the same merchant is served.
		r.orch.SetRouting(fixedRouting{gw: "ids"})
		out, err := r.orch.Create(ctx, r.req("key-fallback-04", 10000))
		if err != nil || out.Payment.Gateway != "ids" {
			t.Fatalf("%+v %v", out.Payment, err)
		}
	})
}

// A payment created through a platform gateway has no merchant_gateway_accounts row, yet the
// poller / "Cek status" must still be able to ask the gateway about it.
func TestInquireWorksForPlatformGatewayPaymentsWithoutMerchantAccount(t *testing.T) {
	r := rsetup(t)
	r.register()
	ctx := context.Background()
	r.orch.SetRouting(fixedRouting{gw: "ids"})
	out, err := r.orch.Create(ctx, r.req("key-inquire-001", 10000))
	if err != nil || out.Payment.Gateway != "ids" {
		t.Fatalf("%+v %v", out.Payment, err)
	}
	var sawRef atomic.Value
	r.ids.query = func(q gateway.QueryRequest) (gateway.QueryResult, error) {
		sawRef.Store(q.AccountRef)
		return gateway.QueryResult{RawStatus: "PAID", Amount: 10000, Currency: "IDR"}, nil
	}
	if err := r.orch.Inquire(ctx, out.Payment.ID); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Get(ctx, r.mid, out.Payment.ID)
	if err != nil || got.Status != Succeeded {
		t.Fatalf("after inquiry: %+v %v", got, err)
	}
	if ref, _ := sawRef.Load().(string); ref != "" {
		t.Fatalf("inquiry sent account ref %q", ref)
	}
	if n := r.count(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'payment_succeeded' AND source_id = $1`, out.Payment.ID); n != 1 {
		t.Fatalf("journals = %d", n)
	}
}

// A gateway that DOES need the merchant's account still reports "not found" when that account is gone.
func TestInquireStillNeedsTheAccountForAccountGateways(t *testing.T) {
	r := rsetup(t)
	ctx := context.Background()
	out, err := r.orch.Create(ctx, r.req("key-inquire-002", 10000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(ctx, `DELETE FROM merchant_gateway_accounts`); err != nil {
		t.Fatal(err)
	}
	if err := r.orch.Inquire(ctx, out.Payment.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
