package settlement

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// sandboxTwin turns merchant b into a's sandbox twin (before it has any history).
func sandboxTwin(t *testing.T, e env) {
	t.Helper()
	if _, err := e.db.Exec(context.Background(), `UPDATE merchants SET is_sandbox = true, parent_id = $1 WHERE id = $2`, e.a, e.b); err != nil {
		t.Fatal(err)
	}
}

// Sandbox payments are never settled against real gateway reports: not by the service, not in parallel,
// not by a direct write, and Finance's unsettled list never shows them.
func TestSandboxPaymentsAreNeverSettled(t *testing.T) {
	e := setup(t)
	sandboxTwin(t, e)
	ctx := context.Background()
	live := e.ok(t, e.a, "gw1", "L1", 100_000)
	sbx := e.ok(t, e.b, "gw1", "S1", 100_000)

	before := e.n(t, `SELECT count(*) FROM ledger_transactions`)
	if _, err := e.svc.RecordBatch(ctx, e.batch("B-MIX", net(100_000)+net(100_000), live, sbx), Actor{}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("batch with a sandbox payment: %v", err)
	}
	if _, err := e.svc.RecordBatch(ctx, e.batch("B-SBX", net(100_000), sbx), Actor{}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("batch of a sandbox payment: %v", err)
	}
	if e.n(t, `SELECT count(*) FROM settlement_batches`) != 0 || e.n(t, `SELECT count(*) FROM ledger_transactions`) != before {
		t.Fatal("a refused batch left a batch or a journal behind")
	}

	// Concurrent attempts: every one refused, nothing booked.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.RecordBatch(ctx, e.batch("B-RACE", net(100_000), sbx), Actor{}); !errors.Is(err, ErrIneligible) {
				t.Errorf("concurrent: %v", err)
			}
		}()
	}
	wg.Wait()
	if e.n(t, `SELECT count(*) FROM settlement_batches`) != 0 {
		t.Fatal("a concurrent attempt created a batch")
	}

	// The real payment still settles on its own, and the books stay in the live world.
	if _, err := e.svc.RecordBatch(ctx, e.batch("B-LIVE", net(100_000), live), Actor{}); err != nil {
		t.Fatal(err)
	}
	if e.n(t, `SELECT count(*) FROM ledger_entries le JOIN ledger_accounts a ON a.id = le.account_id
		WHERE a.code = 'cash_bank_pt' AND NOT a.livemode`) != 0 {
		t.Fatal("settlement touched sandbox books")
	}

	// Finance's queue lists the real payment only... but the sandbox merchant's own view still has its payment.
	list, err := e.svc.ListUnsettled(ctx, "", "", 0)
	if err != nil || len(list) != 0 {
		t.Fatalf("platform unsettled after settling the live one: %+v %v", list, err)
	}
	e.ok(t, e.a, "gw1", "L2", 50_000)
	list, _ = e.svc.ListUnsettled(ctx, "", "", 0)
	if len(list) != 1 || list[0].MerchantID != e.a {
		t.Fatalf("platform unsettled: %+v", list)
	}
	if own, _ := e.svc.ListUnsettledFor(ctx, e.b, "", 0); len(own) != 1 {
		t.Fatalf("sandbox merchant's own unsettled: %+v", own)
	}

	// Direct write: the database refuses a settlement item for a sandbox merchant.
	var batch string
	if err := e.db.QueryRow(ctx, `SELECT id::text FROM settlement_batches WHERE batch_ref = 'B-LIVE'`).Scan(&batch); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(ctx, `INSERT INTO settlement_items (batch_id, payment_id, merchant_id, gross, mdr_fee, expected_net)
		VALUES ($1,$2,$3,100000,700,99300)`, batch, sbx, e.b); err == nil {
		t.Fatal("database accepted a sandbox settlement item")
	}
}

// A sandbox merchant is never settled, so its money must be available the moment a payment succeeds
// (otherwise sandbox payouts and refunds could never be tried). A real merchant still waits for settlement.
func TestSandboxFundsAreAvailableAtOnce(t *testing.T) {
	e := setup(t)
	sandboxTwin(t, e)
	e.ok(t, e.b, "gw1", "S1", 100_000)
	e.ok(t, e.a, "gw1", "L1", 100_000)
	sb, lb := e.bal(t, e.b), e.bal(t, e.a)
	if sb.Pending != 0 || sb.Available != sb.Payable || sb.Available == 0 {
		t.Fatalf("sandbox balance: %+v", sb)
	}
	if lb.Pending == 0 || lb.Available != 0 {
		t.Fatalf("live balance before settlement: %+v", lb)
	}
}
