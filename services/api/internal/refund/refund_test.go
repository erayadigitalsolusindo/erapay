package refund

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/settlement"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type env struct {
	db        *pgxpool.Pool
	svc       *Service
	set       *settlement.Service
	pay       *payment.Service
	a, b      string
	ua, ub    string
	fin, fin2 string
	n         int
}

func setup(t *testing.T) *env {
	t.Helper()
	db := testdb.New(t)
	ctx := context.Background()
	a, _ := testdb.SeedMerchant(t, db, "A")
	b, _ := testdb.SeedMerchant(t, db, "B")
	var plan string
	if err := db.QueryRow(ctx, `INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('std', 1, 'gw1', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{a, b} {
		if _, err := db.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from) VALUES ($1,$2, now() - interval '1 day')`, m, plan); err != nil {
			t.Fatal(err)
		}
	}
	au := auth.NewService(db, auth.DefaultConfig())
	user := func(email string) string {
		id, err := au.CreateUser(ctx, email, "correct horse battery")
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return &env{db: db, svc: NewService(db), set: settlement.NewService(db), pay: payment.NewService(db), a: a, b: b,
		ua: user("ua@x.test"), ub: user("ub@x.test"), fin: user("fin@x.test"), fin2: user("fin2@x.test")}
}

// paid creates a succeeded payment; settled=true also settles it so its money is `available`.
func (e *env) paid(t *testing.T, m string, gross int64, settled bool) string {
	t.Helper()
	ctx := context.Background()
	e.n++
	tx, _ := e.db.Begin(ctx)
	p, err := e.pay.CreateTx(ctx, tx, payment.CreateInput{MerchantID: m, MerchantReference: "R" + strconv.Itoa(e.n), Amount: gross,
		Currency: "IDR", PaymentMethod: "QRIS", Gateway: "gw1"})
	if err != nil {
		t.Fatal(err)
	}
	tx.Commit(ctx) //nolint:errcheck
	for _, to := range []payment.Status{payment.Pending, payment.Succeeded} {
		src := payment.SourceInternal
		if to == payment.Succeeded {
			src = payment.SourceWebhook
		}
		if _, err := e.pay.Transition(ctx, payment.TransitionInput{PaymentID: p.ID, To: to, Source: src}); err != nil {
			t.Fatal(err)
		}
	}
	if settled {
		if _, err := e.set.RecordBatch(ctx, settlement.BatchInput{Gateway: "gw1", BatchRef: "S-" + strconv.Itoa(e.n), Currency: "IDR",
			SettlementDate: time.Now(), ReportedNet: gross - p.Fee.MDRFee, PaymentIDs: []string{p.ID}}, settlement.Actor{}); err != nil {
			t.Fatal(err)
		}
	}
	return p.ID
}

func (e *env) req(m, maker, payID string, amount int64, key string) (Refund, bool, error) {
	body := fmt.Sprintf(`{"p":%q,"n":%d}`, payID, amount)
	h, _ := idempotency.HashJSON([]byte(body))
	return e.svc.Request(context.Background(), RequestInput{MerchantID: m, RequestedBy: maker, PaymentID: payID, Amount: amount,
		Reason: "customer changed their mind", IdempotencyKey: key, RequestHash: h})
}

func (e *env) bal(t *testing.T, m string) settlement.Balance {
	t.Helper()
	b, err := e.set.Balance(context.Background(), m, "IDR")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func (e *env) count(t *testing.T, q string, args ...any) (n int64) {
	t.Helper()
	if err := e.db.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) booksBalance(t *testing.T) {
	t.Helper()
	if e.count(t, `SELECT count(*) FROM (SELECT txn_id FROM ledger_entries GROUP BY txn_id
		HAVING SUM(CASE direction WHEN 'debit' THEN amount ELSE -amount END) <> 0) x`) != 0 {
		t.Fatal("unbalanced journal")
	}
}

func TestRefundLifecycleMovesMoneyThroughTheLedger(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	pid := e.paid(t, e.a, 1_000_000, true)
	start := e.bal(t, e.a)

	r, replayed, err := e.req(e.a, e.ua, pid, 300_000, "key-refund-0001")
	if err != nil || replayed || r.Status != "pending_approval" {
		t.Fatalf("request: %+v %v", r, err)
	}
	if b := e.bal(t, e.a); b.OnHold != 300_000 || b.Available != start.Available-300_000 || b.Payable != start.Payable-300_000 {
		t.Fatalf("after request: %+v (start %+v)", b, start)
	}
	// The payment itself is untouched: still succeeded, with the refund shown separately.
	p, _ := e.pay.Get(ctx, e.a, pid)
	if p.Status != payment.Succeeded || p.RefundingAmount != 300_000 || p.RefundedAmount != 0 {
		t.Fatalf("payment view: %+v", p)
	}

	if _, err := e.svc.MarkRefunded(ctx, r.ID, "BANK-RF-1", Actor{UserID: e.fin}); !errors.Is(err, ErrState) {
		t.Fatalf("mark-refunded before approval: %v", err)
	}
	if _, err := e.svc.Approve(ctx, r.ID, Actor{UserID: e.ua}); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self approval: %v", err)
	}
	if ap, err := e.svc.Approve(ctx, r.ID, Actor{UserID: e.fin}); err != nil || ap.Status != "approved" {
		t.Fatalf("approve: %+v %v", ap, err)
	}
	if _, err := e.svc.Approve(ctx, r.ID, Actor{UserID: e.fin2}); !errors.Is(err, ErrState) {
		t.Fatalf("second checker: %v", err)
	}
	done, err := e.svc.MarkRefunded(ctx, r.ID, "BANK-RF-1", Actor{UserID: e.fin})
	if err != nil || done.Status != "refunded" || done.RefundedAt == nil {
		t.Fatalf("refunded: %+v %v", done, err)
	}
	if _, err := e.svc.MarkRefunded(ctx, r.ID, "BANK-RF-1", Actor{UserID: e.fin}); err != nil {
		t.Fatalf("repeat same ref: %v", err)
	}
	if _, err := e.svc.MarkRefunded(ctx, r.ID, "OTHER", Actor{UserID: e.fin}); !errors.Is(err, ErrRefConflict) {
		t.Fatalf("repeat other ref: %v", err)
	}
	if _, err := e.svc.MarkFailed(ctx, r.ID, "too late to fail", Actor{UserID: e.fin}); !errors.Is(err, ErrState) {
		t.Fatalf("fail after refunded: %v", err)
	}

	end := e.bal(t, e.a)
	if end.OnHold != 0 || end.Payable != start.Payable-300_000 || end.Available != start.Available-300_000 {
		t.Fatalf("end: %+v", end)
	}
	p, _ = e.pay.Get(ctx, e.a, pid)
	if p.Status != payment.Succeeded || p.RefundedAmount != 300_000 || p.RefundingAmount != 0 {
		t.Fatalf("payment after refund: %+v", p)
	}
	if e.count(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'refund_paid'`) != 1 {
		t.Fatal("paid journal posted more than once")
	}
	if e.count(t, `SELECT count(*) FROM webhook_events WHERE type = 'refund.refunded'`) != 1 {
		t.Fatal("merchant webhook event missing or duplicated")
	}
	e.booksBalance(t)
}

func TestRefundCapsAndEligibility(t *testing.T) {
	e := setup(t)
	pid := e.paid(t, e.a, 1_000_000, true)
	e.paid(t, e.a, 1_000_000, true) // plenty of balance, so only the per-payment cap can refuse

	if _, _, err := e.req(e.a, e.ua, pid, 600_000, "key-cap-00001"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.req(e.a, e.ua, pid, 400_001, "key-cap-00002"); !errors.Is(err, ErrTooMuch) {
		t.Fatalf("over the remaining amount: %v", err)
	}
	if _, _, err := e.req(e.a, e.ua, pid, 400_000, "key-cap-00003"); err != nil {
		t.Fatalf("exactly the remainder: %v", err)
	}
	if _, _, err := e.req(e.a, e.ua, pid, 1, "key-cap-00004"); !errors.Is(err, ErrTooMuch) {
		t.Fatalf("nothing left: %v", err)
	}

	// Not succeeded, not mine, malformed.
	ctx := context.Background()
	tx, _ := e.db.Begin(ctx)
	pend, _ := e.pay.CreateTx(ctx, tx, payment.CreateInput{MerchantID: e.a, MerchantReference: "P", Amount: 50_000, Currency: "IDR", PaymentMethod: "QRIS", Gateway: "gw1"})
	tx.Commit(ctx)                                                                                                          //nolint:errcheck
	e.pay.Transition(ctx, payment.TransitionInput{PaymentID: pend.ID, To: payment.Pending, Source: payment.SourceInternal}) //nolint:errcheck
	if _, _, err := e.req(e.a, e.ua, pend.ID, 10_000, "key-elig-0001"); !errors.Is(err, ErrNotRefundable) {
		t.Fatalf("pending payment: %v", err)
	}
	e.paid(t, e.b, 500_000, true)
	if _, _, err := e.req(e.b, e.ub, pid, 10_000, "key-elig-0002"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another merchant's payment: %v", err)
	}
	if _, _, err := e.req(e.a, e.ua, "not-a-uuid", 10_000, "key-elig-0003"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed id: %v", err)
	}
	if _, _, err := e.req(e.a, e.ua, pid, 0, "key-elig-0004"); !errors.Is(err, ErrValidation) {
		t.Fatalf("zero amount: %v", err)
	}
}

func TestRefundOnlyFromSettledFundsAndNeverNegative(t *testing.T) {
	e := setup(t)
	unsettled := e.paid(t, e.a, 1_000_000, false)
	if _, _, err := e.req(e.a, e.ua, unsettled, 100_000, "key-bal-00001"); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("refund of unsettled money: %v", err)
	}
	// Settled money exists, but a payout-sized hole in it must also stop the refund.
	pid := e.paid(t, e.a, 200_000, true)
	avail := e.bal(t, e.a).Available
	if _, _, err := e.req(e.a, e.ua, pid, 200_000, "key-bal-00002"); !errors.Is(err, ErrInsufficient) && avail < 200_000 {
		t.Fatalf("refund above available: %v", err)
	}
	if b := e.bal(t, e.a); b.Available < 0 || b.Payable < 0 || b.OnHold < 0 {
		t.Fatalf("negative balance: %+v", b)
	}
}

// Racing refunds on one payment: the cap holds and the money is reserved exactly once per accepted refund.
func TestConcurrentRefundsRespectTheCap(t *testing.T) {
	e := setup(t)
	pid := e.paid(t, e.a, 1_000_000, true)
	e.paid(t, e.a, 2_000_000, true)
	var ok, capped atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.req(e.a, e.ua, pid, 300_000, "key-race-"+strconv.Itoa(i)+"-0000")
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrTooMuch):
				capped.Add(1)
			default:
				t.Errorf("request: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 || capped.Load() != 9 { // 3 x 300k fit in 1M, a 4th does not
		t.Fatalf("ok=%d capped=%d", ok.Load(), capped.Load())
	}
	if b := e.bal(t, e.a); b.OnHold != 900_000 {
		t.Fatalf("on hold: %+v", b)
	}
	e.booksBalance(t)
}

func TestEndingARefundReleasesTheReservation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	pid := e.paid(t, e.a, 1_000_000, true)
	e.paid(t, e.a, 1_000_000, true) // extra settled money: fees are not refunded, so a full-gross refund needs headroom
	start := e.bal(t, e.a)
	released := func(name string) {
		t.Helper()
		if b := e.bal(t, e.a); b != start {
			t.Fatalf("%s: %+v want %+v", name, b, start)
		}
	}
	r1, _, _ := e.req(e.a, e.ua, pid, 400_000, "key-end-000001")
	if c, err := e.svc.Cancel(ctx, e.a, r1.ID, Actor{UserID: e.ua}); err != nil || c.Status != "cancelled" {
		t.Fatalf("cancel: %+v %v", c, err)
	}
	released("cancel")
	if _, err := e.svc.Cancel(ctx, e.b, r1.ID, Actor{UserID: e.ub}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another tenant cancels: %v", err)
	}

	r2, _, _ := e.req(e.a, e.ua, pid, 400_000, "key-end-000002")
	if _, err := e.svc.Reject(ctx, r2.ID, "no", Actor{UserID: e.fin}); !errors.Is(err, ErrValidation) {
		t.Fatalf("reject without note: %v", err)
	}
	if _, err := e.svc.Reject(ctx, r2.ID, "outside the refund window", Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	released("reject")

	r3, _, _ := e.req(e.a, e.ua, pid, 400_000, "key-end-000003")
	e.svc.Approve(ctx, r3.ID, Actor{UserID: e.fin}) //nolint:errcheck
	if f, err := e.svc.MarkFailed(ctx, r3.ID, "card issuer declined the return", Actor{UserID: e.fin}); err != nil || f.Status != "failed" {
		t.Fatalf("mark failed: %+v %v", f, err)
	}
	released("failed")
	if _, err := e.svc.Cancel(ctx, e.a, r3.ID, Actor{UserID: e.ua}); !errors.Is(err, ErrState) {
		t.Fatalf("cancel after failed: %v", err)
	}
	// The cap is restored too: the full amount is refundable again.
	if _, _, err := e.req(e.a, e.ua, pid, 1_000_000, "key-end-000004"); err != nil {
		t.Fatalf("full refund after releases: %v", err)
	}
	if e.count(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'refund_released'`) != 3 {
		t.Fatal("want exactly three release journals")
	}
	e.booksBalance(t)
}

func TestRefundRequestIsIdempotent(t *testing.T) {
	e := setup(t)
	pid := e.paid(t, e.a, 1_000_000, true)
	first, replayed, err := e.req(e.a, e.ua, pid, 100_000, "key-idem-00001")
	if err != nil || replayed {
		t.Fatal(err)
	}
	again, replayed, err := e.req(e.a, e.ua, pid, 100_000, "key-idem-00001")
	if err != nil || !replayed || again.ID != first.ID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if _, _, err := e.req(e.a, e.ua, pid, 100_001, "key-idem-00001"); !errors.Is(err, idempotency.ErrConflict) {
		t.Fatalf("same key other body: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := e.req(e.a, e.ua, pid, 50_000, "key-storm-0001"); err != nil && !errors.Is(err, idempotency.ErrInProgress) {
				t.Errorf("storm: %v", err)
			}
		}()
	}
	wg.Wait()
	if e.count(t, `SELECT count(*) FROM refunds WHERE amount = 50000`) != 1 || e.bal(t, e.a).OnHold != 150_000 {
		t.Fatal("a retry storm reserved more than once")
	}
}

func TestDatabaseGuardsRefunds(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	pid := e.paid(t, e.a, 1_000_000, true)
	r, _, _ := e.req(e.a, e.ua, pid, 100_000, "key-guard-0001")
	for name, q := range map[string]string{
		"maker approves own refund": fmt.Sprintf(`UPDATE refunds SET status='approved', approved_by=requested_by, approved_at=now() WHERE id='%s'`, r.ID),
		"approved without checker":  fmt.Sprintf(`UPDATE refunds SET status='approved' WHERE id='%s'`, r.ID),
		"refunded without proof":    fmt.Sprintf(`UPDATE refunds SET status='refunded', refunded_at=now() WHERE id='%s'`, r.ID),
		"change amount":             fmt.Sprintf(`UPDATE refunds SET amount = amount + 1 WHERE id='%s'`, r.ID),
		"change payment":            fmt.Sprintf(`UPDATE refunds SET payment_id = gen_random_uuid() WHERE id='%s'`, r.ID),
		"delete":                    fmt.Sprintf(`DELETE FROM refunds WHERE id='%s'`, r.ID),
	} {
		if _, err := e.db.Exec(ctx, q); err == nil {
			t.Errorf("%s: database allowed it", name)
		}
	}
	e.svc.Cancel(ctx, e.a, r.ID, Actor{UserID: e.ua}) //nolint:errcheck
	if _, err := e.db.Exec(ctx, `UPDATE refunds SET status='pending_approval' WHERE id=$1`, r.ID); err == nil {
		t.Error("a cancelled refund was revived")
	}
}
