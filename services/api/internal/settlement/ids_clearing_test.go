package settlement

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
)

func (e env) idsBatch(ref string, reportedNet int64, ids ...string) BatchInput {
	in := e.batch(ref, reportedNet, ids...)
	in.Gateway = "ids"
	return in
}

// IDS holds its own money: its payments and settlements move cash_ids_clearing, and the DOKU
// clearing account is left exactly as DOKU alone would have left it.
func TestIDSPaymentsBookToIDSClearingAndSettleThere(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	i1, i2 := e.ok(t, e.a, "ids", "I1", 100000), e.ok(t, e.a, "ids", "I2", 50000)
	d1 := e.ok(t, e.a, "gw1", "D1", 20000)
	e.paid(t, e.a, "ids", "I3", 77000, payment.Pending) // not paid: must never count

	if got := e.account(t, "cash_ids_clearing"); got != net(100000)+net(50000) {
		t.Fatalf("cash_ids_clearing = %d", got)
	}
	if got := e.account(t, "cash_doku_clearing"); got != net(20000) {
		t.Fatalf("cash_doku_clearing = %d, want only the DOKU payment %d", got, net(20000))
	}

	// Idempotent: the same success reported again posts nothing.
	for i := 0; i < 3; i++ {
		r, err := e.pay.Transition(ctx, payment.TransitionInput{PaymentID: i1, To: payment.Succeeded, Source: payment.SourceWebhook})
		if err != nil || r.Applied || !r.NoOp {
			t.Fatalf("repeat %d: %+v %v", i, r, err)
		}
	}
	if e.n(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'payment_succeeded' AND source_id = $1`, i1) != 1 {
		t.Fatal("payment posted more than once")
	}

	// A DOKU batch cannot take an IDS payment, and an IDS batch cannot take a DOKU one.
	if _, err := e.svc.RecordBatch(ctx, e.batch("WRONG-1", net(100000), i1), Actor{}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("ids payment in a gw1 batch: %v", err)
	}
	if _, err := e.svc.RecordBatch(ctx, e.idsBatch("WRONG-2", net(20000), d1), Actor{}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("gw1 payment in an ids batch: %v", err)
	}

	want := net(100000) + net(50000)
	b, err := e.svc.RecordBatch(ctx, e.idsBatch("IDS-001", want, i1, i2), Actor{})
	if err != nil || b.Replayed || b.ExpectedNet != want {
		t.Fatalf("batch: %+v %v", b, err)
	}
	if got := e.account(t, "cash_ids_clearing"); got != 0 {
		t.Fatalf("cash_ids_clearing not drained: %d", got)
	}
	if got := e.account(t, "cash_doku_clearing"); got != net(20000) {
		t.Fatalf("cash_doku_clearing moved by an IDS settlement: %d", got)
	}
	if got := e.account(t, "cash_bank_pt"); got != want {
		t.Fatalf("cash_bank_pt = %d, want %d", got, want)
	}

	// The same batch again replays: nothing is booked twice.
	again, err := e.svc.RecordBatch(ctx, e.idsBatch("IDS-001", want, i2, i1), Actor{})
	if err != nil || !again.Replayed || again.ID != b.ID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if e.n(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'settlement_batch'`) != 1 ||
		e.account(t, "cash_bank_pt") != want || e.account(t, "cash_ids_clearing") != 0 {
		t.Fatal("replay changed the books")
	}

	// A wrong reported figure books nothing, exactly as for DOKU.
	i4 := e.ok(t, e.a, "ids", "I4", 10000)
	if _, err := e.svc.RecordBatch(ctx, e.idsBatch("IDS-BAD", net(10000)-1, i4), Actor{}); err == nil {
		t.Fatal("mismatching IDS batch was booked")
	}
	if e.account(t, "cash_ids_clearing") != net(10000) {
		t.Fatal("mismatch moved the clearing account")
	}
	if e.n(t, `SELECT count(*) FROM (SELECT txn_id FROM ledger_entries GROUP BY txn_id
		HAVING SUM(CASE direction WHEN 'debit' THEN amount ELSE -amount END) <> 0) x`) != 0 {
		t.Fatal("unbalanced journal")
	}
}

// The same success arriving from several places at once (webhook retries, the inquiry poller)
// is applied exactly once: one status change, one journal, one clearing debit.
func TestConcurrentSuccessOnIDSPostsOnce(t *testing.T) {
	e := setup(t)
	for round := 0; round < 5; round++ {
		id := e.paid(t, e.a, "ids", "R"+string(rune('A'+round)), 123456, payment.Pending)
		// paid(..., Pending) leaves the payment in `created`; the gateway accepting the QR makes it pending.
		if _, err := e.pay.Transition(context.Background(), payment.TransitionInput{
			PaymentID: id, To: payment.Pending, Source: payment.SourceGatewayResponse}); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var applied atomic.Int32
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := e.pay.Transition(context.Background(), payment.TransitionInput{
					PaymentID: id, To: payment.Succeeded, Source: payment.SourceWebhook})
				if err != nil {
					t.Errorf("transition: %v", err)
					return
				}
				if r.Applied {
					applied.Add(1)
				}
			}()
		}
		wg.Wait()
		if applied.Load() != 1 {
			t.Fatalf("round %d: applied %d times", round, applied.Load())
		}
		if e.n(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'payment_succeeded' AND source_id = $1`, id) != 1 {
			t.Fatalf("round %d: journal posted more than once", round)
		}
	}
	if got := e.account(t, "cash_ids_clearing"); got != 5*net(123456) {
		t.Fatalf("cash_ids_clearing = %d, want %d", got, 5*net(123456))
	}
	if got := e.account(t, "cash_doku_clearing"); got != 0 {
		t.Fatalf("cash_doku_clearing = %d, want 0", got)
	}
}

// Identical IDS submissions racing: one batch, one journal, the clearing account drained once.
func TestConcurrentIDSBatchSubmissionsBookOnce(t *testing.T) {
	e := setup(t)
	var ids []string
	var total int64
	for i := 0; i < 15; i++ {
		g := 10000 + int64(i)*1000
		ids = append(ids, e.ok(t, e.a, "ids", "O"+string(rune('A'+i)), g))
		total += net(g)
	}
	var wg sync.WaitGroup
	var created, replayed atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := e.svc.RecordBatch(context.Background(), e.idsBatch("IDS-RACE", total, ids...), Actor{})
			switch {
			case err != nil:
				t.Errorf("submit: %v", err)
			case b.Replayed:
				replayed.Add(1)
			default:
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 || replayed.Load() != 11 {
		t.Fatalf("created=%d replayed=%d", created.Load(), replayed.Load())
	}
	if e.n(t, `SELECT count(*) FROM ledger_transactions WHERE source_type='settlement_batch'`) != 1 ||
		e.account(t, "cash_bank_pt") != total || e.account(t, "cash_ids_clearing") != 0 {
		t.Fatal("books differ from a single booking")
	}
}
