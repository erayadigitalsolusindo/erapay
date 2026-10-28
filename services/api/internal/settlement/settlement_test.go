package settlement

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type env struct {
	db   *pgxpool.Pool
	pay  *payment.Service
	svc  *Service
	a, b string // merchants
}

func setup(t *testing.T) env {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	a, _ := testdb.SeedMerchant(t, db, "A")
	b, _ := testdb.SeedMerchant(t, db, "B")
	for _, gw := range []string{"gw1", "gw2", "ids"} {
		var plan string
		if err := db.QueryRow(ctx, `
			INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
			VALUES ('std-'||$1::text, 1, $1, 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`, gw).Scan(&plan); err != nil {
			t.Fatal(err)
		}
		for _, m := range []string{a, b} {
			if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
				VALUES ($1,$2, now() - interval '1 day')`, m, plan); err != nil {
				t.Fatal(err)
			}
		}
	}
	return env{db, payment.NewService(db), NewService(db), a, b}
}

// paid creates a payment and drives it to the given final-or-not status through the real service.
func (e env) paid(t *testing.T, merchant, gw, ref string, amount int64, final payment.Status) string {
	t.Helper()
	ctx := context.Background()
	tx, _ := e.db.Begin(ctx)
	defer tx.Rollback(ctx) //nolint:errcheck
	p, err := e.pay.CreateTx(ctx, tx, payment.CreateInput{MerchantID: merchant, MerchantReference: ref, Amount: amount,
		Currency: "IDR", PaymentMethod: "QRIS", Gateway: gw})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, to := range []payment.Status{payment.Pending, final} {
		if to == payment.Pending && final == payment.Pending {
			continue
		}
		src := payment.SourceInternal
		if to != payment.Pending {
			src = payment.SourceWebhook
		}
		if _, err := e.pay.Transition(ctx, payment.TransitionInput{PaymentID: p.ID, To: to, Source: src}); err != nil {
			t.Fatal(err)
		}
	}
	return p.ID
}

func (e env) ok(t *testing.T, merchant, gw, ref string, amount int64) string {
	return e.paid(t, merchant, gw, ref, amount, payment.Succeeded)
}

// net is what the gateway settles for a payment: gross - MDR (0.7% half-up).
func net(gross int64) int64 { return gross - (gross*70+5000)/10000 }

func (e env) batch(ref string, reportedNet int64, ids ...string) BatchInput {
	return BatchInput{Gateway: "gw1", BatchRef: ref, Currency: "IDR", SettlementDate: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		ReportedNet: reportedNet, PaymentIDs: ids}
}

func (e env) n(t *testing.T, q string, args ...any) (n int64) {
	t.Helper()
	if err := e.db.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) bal(t *testing.T, m string) Balance {
	t.Helper()
	b, err := e.svc.Balance(context.Background(), m, "IDR")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e env) account(t *testing.T, code string) int64 {
	return e.n(t, `SELECT COALESCE(SUM(debit_minus_credit),0) FROM ledger_account_balances WHERE code = $1`, code)
}

func TestSettlementMakesFundsAvailableAndBooksLedger(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p1, p2, p3 := e.ok(t, e.a, "gw1", "O1", 100000), e.ok(t, e.a, "gw1", "O2", 50000), e.ok(t, e.a, "gw1", "O3", 25000)
	pb := e.ok(t, e.b, "gw1", "OB", 10000)
	e.paid(t, e.a, "gw1", "O4", 77000, payment.Pending) // not paid: must never count

	merchantNet := func(g int64) int64 { return g - (g*70+5000)/10000 - (g*100+5000)/10000 }
	ba := e.bal(t, e.a)
	wantPayable := merchantNet(100000) + merchantNet(50000) + merchantNet(25000)
	if ba.Payable != wantPayable || ba.Pending != wantPayable || ba.Available != 0 {
		t.Fatalf("before settlement: %+v want payable=pending=%d available=0", ba, wantPayable)
	}

	// First batch settles p1 + p2 (+ merchant B's payment: batches are per gateway, not per merchant).
	b1, err := e.svc.RecordBatch(ctx, e.batch("B-001", net(100000)+net(50000)+net(10000), p1, p2, pb), Actor{})
	if err != nil || b1.PaymentCount != 3 || b1.Replayed {
		t.Fatalf("batch: %+v %v", b1, err)
	}
	ba = e.bal(t, e.a)
	wantPending := merchantNet(25000)
	if ba.Pending != wantPending || ba.Available != wantPayable-wantPending || ba.Payable != wantPayable {
		t.Fatalf("after batch 1: %+v want pending=%d", ba, wantPending)
	}
	if bb := e.bal(t, e.b); bb.Pending != 0 || bb.Available != bb.Payable || bb.Payable == 0 {
		t.Fatalf("merchant B: %+v", bb)
	}

	// Ledger: bank received exactly the reported net; the DOKU clearing account is drained by the same amount.
	if got := e.account(t, "cash_bank_pt"); got != net(100000)+net(50000)+net(10000) {
		t.Fatalf("cash_bank_pt = %d", got)
	}
	if got := e.account(t, "cash_doku_clearing"); got != net(25000) { // only p3 still awaits settlement
		t.Fatalf("cash_doku_clearing = %d, want %d", got, net(25000))
	}
	if e.n(t, `SELECT count(*) FROM (SELECT txn_id FROM ledger_entries GROUP BY txn_id
		HAVING SUM(CASE direction WHEN 'debit' THEN amount ELSE -amount END) <> 0) x`) != 0 {
		t.Fatal("unbalanced journal")
	}

	if _, err := e.svc.RecordBatch(ctx, e.batch("B-002", net(25000), p3), Actor{}); err != nil {
		t.Fatal(err)
	}
	if ba = e.bal(t, e.a); ba.Pending != 0 || ba.Available != ba.Payable {
		t.Fatalf("fully settled: %+v", ba)
	}
	if e.account(t, "cash_doku_clearing") != 0 {
		t.Fatal("clearing not drained")
	}
}

func TestMismatchBooksNothingAndOpensOneException(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p1, p2 := e.ok(t, e.a, "gw1", "O1", 100000), e.ok(t, e.a, "gw1", "O2", 50000)
	before := e.bal(t, e.a)
	want := net(100000) + net(50000)

	for i := 0; i < 3; i++ { // repeated wrong submissions
		_, err := e.svc.RecordBatch(ctx, e.batch("BAD-1", want-1, p1, p2), Actor{})
		var mm *Mismatch
		if !errors.As(err, &mm) || mm.Reported != want-1 || mm.Expected != want {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if e.n(t, `SELECT count(*) FROM settlement_batches`) != 0 || e.n(t, `SELECT count(*) FROM settlement_items`) != 0 ||
		e.n(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'settlement_batch'`) != 0 {
		t.Fatal("a mismatching batch changed the books")
	}
	if after := e.bal(t, e.a); after != before {
		t.Fatalf("balance changed: %+v -> %+v", before, after)
	}
	if e.n(t, `SELECT count(*) FROM reconciliation_exceptions WHERE status = 'open'`) != 1 {
		t.Fatal("want exactly one open exception")
	}
	var diff int64
	e.db.QueryRow(ctx, `SELECT (details->>'difference')::bigint FROM reconciliation_exceptions`).Scan(&diff) //nolint:errcheck
	if diff != -1 {
		t.Fatalf("difference = %d", diff)
	}

	ex, _ := e.svc.ListExceptions(ctx, "open", 10)
	if len(ex) != 1 {
		t.Fatalf("list: %d", len(ex))
	}
	if err := e.svc.ResolveException(ctx, ex[0].ID, "x", Actor{}); !errors.Is(err, ErrValidation) {
		t.Fatalf("short note accepted: %v", err)
	}
	if err := e.svc.ResolveException(ctx, ex[0].ID, "report corrected by DOKU", Actor{}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ResolveException(ctx, ex[0].ID, "again", Actor{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("double resolve: %v", err)
	}
	// With the corrected figure the same batch_ref now books (the failed attempts left no trace).
	if _, err := e.svc.RecordBatch(ctx, e.batch("BAD-1", want, p1, p2), Actor{}); err != nil {
		t.Fatal(err)
	}
}

func TestIneligiblePaymentsAreRefused(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	ok1 := e.ok(t, e.a, "gw1", "O1", 10000)
	pending := e.paid(t, e.a, "gw1", "O2", 10000, payment.Pending)
	failed := e.paid(t, e.a, "gw1", "O3", 10000, payment.Failed)
	other := e.ok(t, e.a, "gw2", "O4", 10000) // settles on another gateway's batch
	done := e.ok(t, e.a, "gw1", "O5", 10000)
	if _, err := e.svc.RecordBatch(ctx, e.batch("OK", net(10000), done), Actor{}); err != nil {
		t.Fatal(err)
	}

	for name, ids := range map[string][]string{
		"pending": {ok1, pending}, "failed": {ok1, failed}, "other gateway": {ok1, other},
		"already settled": {ok1, done}, "unknown id": {ok1, "99999999-9999-4999-8999-999999999999"},
	} {
		_, err := e.svc.RecordBatch(ctx, e.batch("X-"+name, net(10000)*2, ids...), Actor{})
		if !errors.Is(err, ErrIneligible) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.svc.RecordBatch(ctx, e.batch("X", 1, "not-a-uuid"), Actor{}); !errors.Is(err, ErrValidation) {
		t.Errorf("bad uuid: %v", err)
	}
	for _, bad := range []BatchInput{e.batch("", 1, ok1), e.batch("R", 0, ok1), e.batch("R", 1), e.batch("R", 1, ok1, ok1)} {
		if _, err := e.svc.RecordBatch(ctx, bad, Actor{}); !errors.Is(err, ErrValidation) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if e.n(t, `SELECT count(*) FROM settlement_batches`) != 1 || e.n(t, `SELECT count(*) FROM reconciliation_exceptions`) != 0 {
		t.Fatal("refused batches left traces")
	}
}

func TestBatchRefIsIdempotentAndConflictsAreRefused(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p1, p2 := e.ok(t, e.a, "gw1", "O1", 10000), e.ok(t, e.a, "gw1", "O2", 20000)
	in := e.batch("DUP", net(10000)+net(20000), p1, p2)
	first, err := e.svc.RecordBatch(ctx, in, Actor{})
	if err != nil || first.Replayed {
		t.Fatalf("first: %+v %v", first, err)
	}
	in.PaymentIDs = []string{p2, p1} // order must not matter
	again, err := e.svc.RecordBatch(ctx, in, Actor{})
	if err != nil || !again.Replayed || again.ID != first.ID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if e.n(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'settlement_batch'`) != 1 {
		t.Fatal("replay posted twice")
	}
	p3 := e.ok(t, e.a, "gw1", "O3", 5000)
	if _, err := e.svc.RecordBatch(ctx, e.batch("DUP", net(10000)+net(20000)+net(5000), p1, p2, p3), Actor{}); !errors.Is(err, ErrBatchConflict) {
		t.Fatalf("different content under same ref: %v", err)
	}
	if _, err := e.svc.RecordBatch(ctx, e.batch("DUP", net(5000), p3), Actor{}); !errors.Is(err, ErrBatchConflict) {
		t.Fatalf("different payments under same ref: %v", err)
	}
}

// Identical submissions racing (double click, retry storm): one batch, one journal, everyone gets it.
func TestConcurrentIdenticalSubmissionsBookOnce(t *testing.T) {
	e := setup(t)
	var ids []string
	var total int64
	for i := 0; i < 20; i++ {
		ids = append(ids, e.ok(t, e.a, "gw1", "O"+string(rune('A'+i)), 10000+int64(i)*1000))
		total += net(10000 + int64(i)*1000)
	}
	var wg sync.WaitGroup
	var created, replayed atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := e.svc.RecordBatch(context.Background(), e.batch("RACE", total, ids...), Actor{})
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
	if e.n(t, `SELECT count(*) FROM settlement_batches`) != 1 || e.n(t, `SELECT count(*) FROM ledger_transactions WHERE source_type='settlement_batch'`) != 1 ||
		e.account(t, "cash_bank_pt") != total {
		t.Fatal("books differ from a single booking")
	}
}

// Different batches that overlap on a payment: the payment settles exactly once, never twice.
func TestOverlappingBatchesCannotDoubleSettle(t *testing.T) {
	e := setup(t)
	shared := e.ok(t, e.a, "gw1", "S", 10000)
	for round := 0; round < 5; round++ {
		var onlyX, onlyY []string
		var netX, netY int64
		for i := 0; i < 6; i++ {
			x, y := e.ok(t, e.a, "gw1", "X", 10000), e.ok(t, e.a, "gw1", "Y", 10000)
			onlyX, onlyY = append(onlyX, x), append(onlyY, y)
			netX, netY = netX+net(10000), netY+net(10000)
		}
		sharedID := shared
		if round > 0 {
			sharedID = e.ok(t, e.a, "gw1", "S", 10000)
		}
		bx := e.batch("X-"+string(rune('0'+round)), netX+net(10000), append(onlyX, sharedID)...)
		by := e.batch("Y-"+string(rune('0'+round)), netY+net(10000), append(onlyY, sharedID)...)
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, in := range []BatchInput{bx, by} {
			wg.Add(1)
			go func() { defer wg.Done(); _, errs[i] = e.svc.RecordBatch(context.Background(), in, Actor{}) }()
		}
		wg.Wait()
		wins := 0
		for _, err := range errs {
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrIneligible):
			default:
				t.Fatalf("round %d: unexpected error %v", round, err)
			}
		}
		if wins != 1 {
			t.Fatalf("round %d: %d batches won, want exactly 1 (%v)", round, wins, errs)
		}
		if n := e.n(t, `SELECT count(*) FROM settlement_items WHERE payment_id = $1`, sharedID); n != 1 {
			t.Fatalf("round %d: shared payment settled %d times", round, n)
		}
	}
	if e.n(t, `SELECT count(*) FROM (SELECT payment_id FROM settlement_items GROUP BY 1 HAVING count(*) > 1) d`) != 0 {
		t.Fatal("payment in several batches")
	}
}

func TestDatabaseRefusesInconsistentOrMutatedSettlement(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p := e.ok(t, e.a, "gw1", "O1", 10000)
	if _, err := e.db.Exec(ctx, `INSERT INTO settlement_batches (gateway, batch_ref, currency, settlement_date, reported_net, expected_net, payment_count)
		VALUES ('gw1','HAND','IDR', current_date, 100, 99, 1)`); err == nil {
		t.Fatal("db accepted a batch whose reported net differs from expected")
	}
	if _, err := e.svc.RecordBatch(ctx, e.batch("OK", net(10000), p), Actor{}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE settlement_items SET gross = gross + 1`, `DELETE FROM settlement_items`,
		`UPDATE settlement_batches SET reported_net = reported_net`, `DELETE FROM settlement_batches`} {
		if _, err := e.db.Exec(ctx, q); err == nil {
			t.Errorf("mutation allowed: %s", q)
		}
	}
}

func TestListUnsettledAndBatches(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	p1 := e.ok(t, e.a, "gw1", "O1", 10000)
	e.ok(t, e.b, "gw1", "O2", 20000)
	e.ok(t, e.a, "gw2", "O3", 30000)
	e.paid(t, e.a, "gw1", "O4", 40000, payment.Pending)
	if _, err := e.svc.RecordBatch(ctx, e.batch("L1", net(10000), p1), Actor{}); err != nil {
		t.Fatal(err)
	}
	all, err := e.svc.ListUnsettled(ctx, "", "", 100)
	if err != nil || len(all) != 2 {
		t.Fatalf("unsettled all: %d %v", len(all), err)
	}
	gw1, _ := e.svc.ListUnsettled(ctx, "gw1", "IDR", 100)
	if len(gw1) != 1 || gw1[0].ExpectedNet != net(20000) || gw1[0].Gross-gw1[0].MDRFee != gw1[0].ExpectedNet {
		t.Fatalf("gw1 unsettled: %+v", gw1)
	}
	if bs, _ := e.svc.ListBatches(ctx, "gw1", 10); len(bs) != 1 || bs[0].BatchRef != "L1" || bs[0].SettlementDate != "2026-10-12" {
		t.Fatalf("batches: %+v", bs)
	}
}
