package payment

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

func evFor(p Payment, status gateway.Status, ok bool, amount int64, dedup string) gateway.WebhookEvent {
	return gateway.WebhookEvent{PaymentRef: p.ID, RawStatus: "SUCCESS", Status: status, StatusOK: ok,
		Amount: amount, Currency: "IDR", DedupKey: dedup}
}

func (e oenv) status(t *testing.T, id string) string {
	t.Helper()
	var s string
	if err := e.db.QueryRow(context.Background(), `SELECT status FROM payments WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestApplyWebhookSucceedsOnceAndPostsLedgerOnce(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	p := e.pending(t, "key-wh-0001", 10000)

	res, err := e.orch.ApplyWebhook(ctx, "gw1", evFor(p, gateway.StatusSucceeded, true, 10000, "gw1:webhook:r1"))
	if err != nil || !res.Applied || res.Status != Succeeded {
		t.Fatalf("%+v %v", res, err)
	}
	if e.status(t, p.ID) != "succeeded" {
		t.Fatal("not succeeded")
	}
	if n := e.count(t, `SELECT count(*) FROM payment_events WHERE payment_id=$1 AND source='webhook' AND applied`, p.ID); n != 1 {
		t.Fatalf("applied webhook events = %d", n)
	}
	txBefore := e.count(t, `SELECT count(*) FROM ledger_transactions`)
	if txBefore != 1 {
		t.Fatalf("ledger transactions = %d, want 1", txBefore)
	}

	// Same notification again (gateway retry): no second effect.
	again, err := e.orch.ApplyWebhook(ctx, "gw1", evFor(p, gateway.StatusSucceeded, true, 10000, "gw1:webhook:r1"))
	if err != nil || again.Applied || !again.Duplicate {
		t.Fatalf("replay: %+v %v", again, err)
	}
	// A different notification id for the same outcome is also a no-op.
	other, err := e.orch.ApplyWebhook(ctx, "gw1", evFor(p, gateway.StatusSucceeded, true, 10000, "gw1:webhook:r2"))
	if err != nil || other.Applied || !other.NoOp {
		t.Fatalf("second id: %+v %v", other, err)
	}
	if n := e.count(t, `SELECT count(*) FROM ledger_transactions`); n != txBefore {
		t.Fatalf("ledger changed on replay: %d", n)
	}
}

func TestApplyWebhookConcurrentDuplicatesApplyExactlyOnce(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	p := e.pending(t, "key-wh-conc1", 10000)

	var wg sync.WaitGroup
	var mu sync.Mutex
	applied, other := 0, 0
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// half share a dedup key (retries), half have distinct ids (separate notifications)
			key := "gw1:webhook:same"
			if i%2 == 1 {
				key = "gw1:webhook:n" + string(rune('a'+i))
			}
			r, err := e.orch.ApplyWebhook(ctx, "gw1", evFor(p, gateway.StatusSucceeded, true, 10000, key))
			if err != nil {
				t.Errorf("apply: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if r.Applied {
				applied++
			} else {
				other++
			}
		}(i)
	}
	wg.Wait()
	if applied != 1 || other != 23 {
		t.Fatalf("applied=%d other=%d", applied, other)
	}
	if n := e.count(t, `SELECT count(*) FROM ledger_transactions`); n != 1 {
		t.Fatalf("ledger transactions = %d, want 1", n)
	}
	if n := e.count(t, `SELECT count(*) FROM payment_events WHERE payment_id=$1 AND applied`, p.ID); n < 1 {
		t.Fatalf("applied events = %d", n)
	}
}

func TestApplyWebhookRefusesWrongAmountUnmappedAndUnknown(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	p := e.pending(t, "key-wh-guard1", 10000)

	if _, err := e.orch.ApplyWebhook(ctx, "gw1", evFor(p, gateway.StatusSucceeded, true, 9999, "gw1:webhook:a")); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("amount mismatch: %v", err)
	}
	bad := evFor(p, gateway.StatusSucceeded, true, 10000, "gw1:webhook:b")
	bad.Currency = "USD"
	if _, err := e.orch.ApplyWebhook(ctx, "gw1", bad); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("currency mismatch: %v", err)
	}
	if _, err := e.orch.ApplyWebhook(ctx, "gw1", evFor(p, "", false, 10000, "gw1:webhook:c")); err != nil {
		t.Fatalf("unmapped: %v", err)
	}
	if e.status(t, p.ID) != "pending" {
		t.Fatalf("status changed: %s", e.status(t, p.ID))
	}

	unknown := evFor(p, gateway.StatusSucceeded, true, 10000, "gw1:webhook:d")
	unknown.PaymentRef = "00000000-0000-4000-8000-00000000abcd"
	if _, err := e.orch.ApplyWebhook(ctx, "gw1", unknown); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown payment: %v", err)
	}
	unknown.PaymentRef = "not-a-uuid'; DROP TABLE payments;--"
	if _, err := e.orch.ApplyWebhook(ctx, "gw1", unknown); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed ref: %v", err)
	}
	// A payment of another gateway is indistinguishable from a missing one.
	if _, err := e.orch.ApplyWebhook(ctx, "other-gw", evFor(p, gateway.StatusSucceeded, true, 10000, "x:webhook:e")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong gateway: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM ledger_transactions`); n != 0 {
		t.Fatalf("ledger posted for refused webhooks: %d", n)
	}
}

func TestApplyWebhookLateConflictingReportDoesNotFlipFinalStatus(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	p := e.pending(t, "key-wh-flip01", 10000)
	if _, err := e.orch.ApplyWebhook(ctx, "gw1", evFor(p, gateway.StatusSucceeded, true, 10000, "gw1:webhook:ok")); err != nil {
		t.Fatal(err)
	}
	r, err := e.orch.ApplyWebhook(ctx, "gw1", evFor(p, gateway.StatusFailed, true, 10000, "gw1:webhook:late"))
	if err != nil || !r.Conflict || r.Applied {
		t.Fatalf("%+v %v", r, err)
	}
	if e.status(t, p.ID) != "succeeded" {
		t.Fatalf("flipped to %s", e.status(t, p.ID))
	}
}
