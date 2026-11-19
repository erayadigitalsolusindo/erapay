package payout

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/ledger"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/pricing"
)

// sandboxMerchant turns merchant b into a's sandbox twin before it has any money or ledger history.
func sandboxMerchant(t *testing.T, e *env) {
	t.Helper()
	if _, err := e.db.Exec(context.Background(),
		`UPDATE merchants SET is_sandbox = true, parent_id = $1 WHERE id = $2`, e.a, e.b); err != nil {
		t.Fatal(err)
	}
}

// fundSandbox credits the sandbox merchant's balance through the ledger. It deliberately does not go through
// settlement: sandbox payments are never settled against real gateway reports (stage 5 enforces that).
func fundSandbox(t *testing.T, e *env, net int64) {
	t.Helper()
	e.n++
	txn, err := ledger.PaymentSucceeded(e.b, fmt.Sprintf("sbx-fund-%d", e.n), pricing.Breakdown{Gross: net + 10, MDRFee: 0, PlatformFee: 10, MerchantNet: net, Currency: "IDR"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.NewService(e.db).Post(context.Background(), txn); err != nil {
		t.Fatal(err)
	}
}

// approvedSandboxPayout requests (as the sandbox merchant) and approves one payout.
func approvedSandboxPayout(t *testing.T, e *env, amount int64) Payout {
	t.Helper()
	fundSandbox(t, e, amount+10000)
	acct := e.account(t, e.b, e.ub)
	p, _, err := e.req(e.b, e.ub, acct, amount, fmt.Sprintf("sbx-key-%d", e.n))
	if err != nil {
		t.Fatalf("sandbox payout request: %v", err)
	}
	if _, err := e.svc.Approve(context.Background(), p.ID, Actor{UserID: e.fin, RequestID: "t"}); err != nil {
		t.Fatal(err)
	}
	return p
}

// A server that runs real payouts (DryRun off): a sandbox merchant's payout is still a dry run that never
// reaches the gateway, whether the go/no-go gate is open or closed; a real merchant's payout is unaffected.
func TestSandboxPayoutIsAlwaysDryRun(t *testing.T) {
	for _, live := range []bool{true, false} {
		t.Run(fmt.Sprintf("gate_open=%v", live), func(t *testing.T) {
			e := execSetup(t, func(c *Config) { c.LiveEnabled = live })
			sandboxMerchant(t, e)
			dis := &fakeDisburser{code: "doku"}
			e.svc.SetDisburser(dis)
			ctx := context.Background()

			p := approvedSandboxPayout(t, e, 100_000)
			if !p.DryRun {
				t.Fatal("sandbox payout was not recorded as a dry run")
			}
			out, err := e.svc.Execute(ctx, p.ID, Actor{UserID: e.fin})
			if err != nil || out.Status != "submitted" || out.Gateway != "dry-run" {
				t.Fatalf("execute: %+v err=%v", out, err)
			}
			if dis.inquiries.Load()+dis.transfers.Load()+dis.checks.Load() != 0 {
				t.Fatalf("gateway was called for a sandbox payout: inquiry=%d transfer=%d", dis.inquiries.Load(), dis.transfers.Load())
			}
			if _, err := e.svc.MarkPaid(ctx, p.ID, "SBX-REF", Actor{UserID: e.fin}); err != nil {
				t.Fatal(err)
			}
			// The bank leg stays in the sandbox world: no real platform account moved.
			real := e.count(t, `SELECT COALESCE(SUM(le.amount),0) FROM ledger_entries le JOIN ledger_accounts a ON a.id = le.account_id
				WHERE a.code = 'cash_bank_pt' AND a.livemode`)
			sbx := e.count(t, `SELECT COALESCE(SUM(le.amount),0) FROM ledger_entries le JOIN ledger_accounts a ON a.id = le.account_id
				WHERE a.code = 'cash_bank_pt' AND NOT a.livemode`)
			if real != 0 || sbx == 0 {
				t.Fatalf("cash_bank_pt: real=%d sandbox=%d", real, sbx)
			}
		})
	}
}

// With the production gate closed, a real merchant is still refused, a sandbox merchant is not.
func TestProductionGateStillClosedForRealMerchants(t *testing.T) {
	e := execSetup(t, func(c *Config) { c.LiveEnabled = false })
	sandboxMerchant(t, e)
	e.fund(t, e.a, 110_000)
	acct := e.account(t, e.a, e.ua)
	if _, _, err := e.req(e.a, e.ua, acct, 100_000, "real-key-0001"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("real merchant with the gate closed: %v", err)
	}
	fundSandbox(t, e, 110_000)
	sacct := e.account(t, e.b, e.ub)
	if _, _, err := e.req(e.b, e.ub, sacct, 100_000, "sbx-key-0001"); err != nil {
		t.Fatalf("sandbox merchant with the gate closed: %v", err)
	}
}

// Parallel executes of one sandbox payout: one wins, the rest see ErrState, and the gateway is never asked to move money.
func TestSandboxPayoutConcurrentExecute(t *testing.T) {
	e := execSetup(t)
	sandboxMerchant(t, e)
	dis := &fakeDisburser{code: "doku"}
	e.svc.SetDisburser(dis)
	p := approvedSandboxPayout(t, e, 100_000)

	var wg sync.WaitGroup
	var ok, state atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Execute(context.Background(), p.ID, Actor{UserID: e.fin})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrState):
				state.Add(1)
			default:
				t.Errorf("execute: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || state.Load() != 11 || dis.transfers.Load() != 0 {
		t.Fatalf("ok=%d state=%d transfers=%d", ok.Load(), state.Load(), dis.transfers.Load())
	}
}

// The platform payout queue (Finance) never lists a sandbox merchant's payouts; the merchant's own list does.
func TestPlatformPayoutQueueSkipsSandbox(t *testing.T) {
	e := execSetup(t)
	sandboxMerchant(t, e)
	ctx := context.Background()
	approvedSandboxPayout(t, e, 100_000)
	approvedPayout(t, e, 100_000)
	all, err := e.svc.List(ctx, "", "", 50)
	if err != nil || len(all) != 1 || all[0].MerchantID != e.a {
		t.Fatalf("platform queue: %+v %v", all, err)
	}
	if own, _ := e.svc.List(ctx, e.b, "", 50); len(own) != 1 {
		t.Fatalf("sandbox merchant's own payouts: %+v", own)
	}
}
