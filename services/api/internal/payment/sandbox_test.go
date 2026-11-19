package payment

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
)

type senv struct {
	oenv
	sbx      *fakeGW
	twin     string
	twinOutl string
}

// ssetup: a production server with a production adapter, a sandbox adapter, a live merchant and its sandbox twin.
// The live merchant only has a production gateway account; the twin only has a sandbox one.
func ssetup(t *testing.T) senv {
	t.Helper()
	e := setup(t)
	ctx := context.Background()
	prod, sbx := newFake(), newFake()
	prodReg, sbxReg := gateway.NewRegistry(), gateway.NewRegistry()
	prodReg.Register(prod)
	sbxReg.Register(sbx)
	o := NewOrchestrator(e.svc, e.db, idempotency.NewStore(e.db), prodReg, "production", slog.New(slog.NewTextHandler(io.Discard, nil)))
	o.SetSandbox(sbxReg)

	var twin, outlet string
	if err := e.db.QueryRow(ctx, `INSERT INTO merchants (legal_name, display_name, onboarding_status, is_sandbox, parent_id)
		VALUES ('A sbx','A sbx','approved',true,$1) RETURNING id::text`, e.mid).Scan(&twin); err != nil {
		t.Fatal(err)
	}
	if err := e.db.QueryRow(ctx, `INSERT INTO merchant_outlets (merchant_id, name) VALUES ($1,'o') RETURNING id::text`, twin).Scan(&outlet); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
		SELECT $1, plan_id, effective_from FROM merchant_pricing_assignments WHERE merchant_id = $2`, twin, e.mid); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][2]string{{e.mid, "production"}, {twin, "sandbox"}} {
		if _, err := e.db.Exec(ctx, `INSERT INTO merchant_gateway_accounts (merchant_id, gateway, environment, gateway_ref, activation_status)
			VALUES ($1,'gw1',$2,'MID-1','active')`, a[0], a[1]); err != nil {
			t.Fatal(err)
		}
	}
	return senv{oenv{e, o, prod}, sbx, twin, outlet}
}

func (s senv) twinReq(key string) CreateRequest {
	r := s.req(key, 10000)
	r.MerchantID, r.OutletID = s.twin, s.twinOutl
	return r
}

// Each merchant is served only by its own world's adapter.
func TestSandboxMerchantUsesSandboxAdapter(t *testing.T) {
	s := ssetup(t)
	ctx := context.Background()
	if _, err := s.orch.Create(ctx, s.twinReq("key-twin-001")); err != nil {
		t.Fatal(err)
	}
	if s.sbx.createCalls.Load() != 1 || s.gw.createCalls.Load() != 0 {
		t.Fatalf("sandbox=%d prod=%d, want 1/0", s.sbx.createCalls.Load(), s.gw.createCalls.Load())
	}
	if _, err := s.orch.Create(ctx, s.req("key-live-001", 10000)); err != nil {
		t.Fatal(err)
	}
	if s.sbx.createCalls.Load() != 1 || s.gw.createCalls.Load() != 1 {
		t.Fatalf("sandbox=%d prod=%d, want 1/1", s.sbx.createCalls.Load(), s.gw.createCalls.Load())
	}
}

// A sandbox merchant never falls back to a production account, and a live merchant never to a sandbox one.
func TestWorldsDoNotLeak(t *testing.T) {
	s := ssetup(t)
	ctx := context.Background()
	if _, err := s.db.Exec(ctx, `UPDATE merchant_gateway_accounts SET environment = 'production' WHERE merchant_id = $1`, s.twin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.orch.Create(ctx, s.twinReq("key-twin-002")); !errors.Is(err, ErrMethodUnavailable) {
		t.Fatalf("sandbox merchant with only a production account: want ErrMethodUnavailable, got %v", err)
	}
	if _, err := s.db.Exec(ctx, `UPDATE merchant_gateway_accounts SET environment = 'sandbox' WHERE merchant_id = $1`, s.mid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.orch.Create(ctx, s.req("key-live-002", 10000)); !errors.Is(err, ErrMethodUnavailable) {
		t.Fatalf("live merchant with only a sandbox account: want ErrMethodUnavailable, got %v", err)
	}
	if s.sbx.createCalls.Load()+s.gw.createCalls.Load() != 0 {
		t.Fatal("a gateway was called")
	}
}

type countingRouting struct{ n atomic.Int32 }

func (c *countingRouting) Preferred(context.Context) (string, error) { c.n.Add(1); return "", nil }

// The platform's gateway choice is for live merchants only.
func TestPlatformRoutingSkippedForSandbox(t *testing.T) {
	s := ssetup(t)
	rt := &countingRouting{}
	s.orch.SetRouting(rt)
	ctx := context.Background()
	if _, err := s.orch.Create(ctx, s.twinReq("key-twin-003")); err != nil {
		t.Fatal(err)
	}
	if rt.n.Load() != 0 {
		t.Fatal("platform routing consulted for a sandbox merchant")
	}
	if _, err := s.orch.Create(ctx, s.req("key-live-003", 10000)); err != nil {
		t.Fatal(err)
	}
	if rt.n.Load() != 1 {
		t.Fatalf("platform routing calls = %d, want 1", rt.n.Load())
	}
}

// Inquiry for a sandbox payment asks the sandbox adapter.
func TestSandboxInquiryUsesSandboxAdapter(t *testing.T) {
	s := ssetup(t)
	ctx := context.Background()
	out, err := s.orch.Create(ctx, s.twinReq("key-twin-004"))
	if err != nil {
		t.Fatal(err)
	}
	q := func(gateway.QueryRequest) (gateway.QueryResult, error) {
		return gateway.QueryResult{RawStatus: "WAIT", Amount: 10000, Currency: "IDR"}, nil
	}
	s.sbx.query, s.gw.query = q, q
	if err := s.orch.Inquire(ctx, out.Payment.ID); err != nil {
		t.Fatal(err)
	}
	if s.sbx.queryCalls.Load() != 1 || s.gw.queryCalls.Load() != 0 {
		t.Fatalf("sandbox=%d prod=%d, want 1/0", s.sbx.queryCalls.Load(), s.gw.queryCalls.Load())
	}
}

// Same key, same sandbox merchant, in parallel: one gateway call, one payment (idempotency holds per world).
func TestSandboxCreateConcurrentSameKey(t *testing.T) {
	s := ssetup(t)
	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.orch.Create(context.Background(), s.twinReq("key-twin-race")); err != nil && !errors.Is(err, idempotency.ErrInProgress) {
				t.Errorf("create: %v", err)
			}
		}()
	}
	wg.Wait()
	if s.sbx.createCalls.Load() != 1 || s.gw.createCalls.Load() != 0 {
		t.Fatalf("sandbox=%d prod=%d, want 1/0", s.sbx.createCalls.Load(), s.gw.createCalls.Load())
	}
	if n := s.count(t, `SELECT count(*) FROM payments WHERE merchant_id = $1`, s.twin); n != 1 {
		t.Fatalf("payments = %d, want 1", n)
	}
}
