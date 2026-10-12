package payout

import (
	"bytes"
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
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/settlement"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type env struct {
	db        *pgxpool.Pool
	svc       *Service
	set       *settlement.Service
	pay       *payment.Service
	a, b      string // merchants
	ua, ub    string // merchant users (makers)
	fin, fin2 string // finance users (checkers)
	n         int
}

func setup(t *testing.T, mod ...func(*Config)) *env {
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
	key := bytes.Repeat([]byte{3}, 32)
	box, _ := secretbox.New(key)
	cfg := DefaultConfig(true)
	cfg.Cooling = 0
	for _, m := range mod {
		m(&cfg)
	}
	return &env{db: db, svc: NewService(db, box, cfg), set: settlement.NewService(db), pay: payment.NewService(db), a: a, b: b,
		ua: user("ua@x.test"), ub: user("ub@x.test"), fin: user("fin@x.test"), fin2: user("fin2@x.test")}
}

// fund gives merchant m `want` available (settled) money by settling one payment.
func (e *env) fund(t *testing.T, m string, gross int64) (merchantNet int64) {
	t.Helper()
	ctx := context.Background()
	e.n++
	tx, _ := e.db.Begin(ctx)
	p, err := e.pay.CreateTx(ctx, tx, payment.CreateInput{MerchantID: m, MerchantReference: "F" + strconv.Itoa(e.n), Amount: gross,
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
	if _, err := e.set.RecordBatch(ctx, settlement.BatchInput{Gateway: "gw1", BatchRef: "FUND-" + strconv.Itoa(e.n), Currency: "IDR",
		SettlementDate: time.Now(), ReportedNet: gross - p.Fee.MDRFee, PaymentIDs: []string{p.ID}}, settlement.Actor{}); err != nil {
		t.Fatal(err)
	}
	return p.Fee.MerchantNet
}

// account adds and verifies a bank account for merchant m (maker: its user, checker: finance).
func (e *env) account(t *testing.T, m, maker string) string {
	t.Helper()
	ctx := context.Background()
	b, err := e.svc.AddBankAccount(ctx, m, "BCA", "1234567890", "PT Merchant", Actor{UserID: maker})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.VerifyBankAccount(ctx, b.ID, Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	return b.ID
}

func (e *env) req(m, maker, acct string, amount int64, key string) (Payout, bool, error) {
	body := fmt.Sprintf(`{"a":%q,"n":%d}`, acct, amount)
	h, _ := idempotency.HashJSON([]byte(body))
	return e.svc.Request(context.Background(), RequestInput{MerchantID: m, RequestedBy: maker, BankAccountID: acct, Amount: amount,
		Currency: "IDR", IdempotencyKey: key, RequestHash: h})
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

func TestPayoutLifecycleMovesMoneyThroughTheLedger(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	net := e.fund(t, e.a, 1_000_000)
	acct := e.account(t, e.a, e.ua)
	start := e.bal(t, e.a)
	if start.Available != net || start.Payable != net || start.InPayout != 0 {
		t.Fatalf("start: %+v want available=%d", start, net)
	}

	p, replayed, err := e.req(e.a, e.ua, acct, 400_000, "key-lifecycle-1")
	if err != nil || replayed || p.Status != "pending_approval" || !p.DryRun {
		t.Fatalf("request: %+v %v", p, err)
	}
	// Reserved at request time: it already left `available`, and `payable`, and sits in `in_payout`.
	if b := e.bal(t, e.a); b.Available != net-400_000 || b.Payable != net-400_000 || b.InPayout != 400_000 {
		t.Fatalf("after request: %+v", b)
	}

	// Payment must not be markable before approval; the requester cannot approve; then Finance approves.
	if _, err := e.svc.MarkPaid(ctx, p.ID, "BANK-REF-1", Actor{UserID: e.fin}); !errors.Is(err, ErrState) {
		t.Fatalf("mark-paid before approval: %v", err)
	}
	if _, err := e.svc.Approve(ctx, p.ID, Actor{UserID: e.ua}); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self approval: %v", err)
	}
	ap, err := e.svc.Approve(ctx, p.ID, Actor{UserID: e.fin})
	if err != nil || ap.Status != "approved" || ap.ApprovedAt == nil {
		t.Fatalf("approve: %+v %v", ap, err)
	}
	if again, err := e.svc.Approve(ctx, p.ID, Actor{UserID: e.fin}); err != nil || again.Status != "approved" {
		t.Fatalf("approve retry by the same checker must be a no-op: %v", err)
	}
	if _, err := e.svc.Approve(ctx, p.ID, Actor{UserID: e.fin2}); !errors.Is(err, ErrState) {
		t.Fatalf("second checker on an approved payout: %v", err)
	}

	paid, err := e.svc.MarkPaid(ctx, p.ID, "BANK-REF-1", Actor{UserID: e.fin})
	if err != nil || paid.Status != "paid" || paid.PaidAt == nil || *paid.BankReference != "BANK-REF-1" {
		t.Fatalf("paid: %+v %v", paid, err)
	}
	if _, err := e.svc.MarkPaid(ctx, p.ID, "BANK-REF-1", Actor{UserID: e.fin}); err != nil {
		t.Fatalf("repeat with the same reference: %v", err)
	}
	if _, err := e.svc.MarkPaid(ctx, p.ID, "OTHER-REF", Actor{UserID: e.fin}); !errors.Is(err, ErrRefConflict) {
		t.Fatalf("repeat with another reference: %v", err)
	}
	if _, err := e.svc.MarkFailed(ctx, p.ID, "too late to fail", Actor{UserID: e.fin}); !errors.Is(err, ErrState) {
		t.Fatalf("fail after paid: %v", err)
	}

	end := e.bal(t, e.a)
	if end.Payable != net-400_000 || end.InPayout != 0 || end.Available != net-400_000 {
		t.Fatalf("end: %+v", end)
	}
	// The bank leg: PT's bank balance fell by exactly the payout, once.
	if got := e.count(t, `SELECT COALESCE(SUM(debit_minus_credit),0) FROM ledger_account_balances WHERE code = 'cash_bank_pt'`); got != 1_000_000-7_000-400_000 { // settled net received, minus the payout sent
		t.Fatalf("cash_bank_pt = %d", got)
	}
	if e.count(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'payout_paid'`) != 1 {
		t.Fatal("paid journal posted more than once")
	}
	if e.count(t, `SELECT count(*) FROM webhook_events WHERE type = 'payout.paid'`) != 1 {
		t.Fatal("merchant webhook event for the paid payout missing or duplicated")
	}
	e.booksBalance(t)
	if e.count(t, `SELECT count(*) FROM audit_logs WHERE action LIKE 'payout.%'`) != 3 {
		t.Fatal("request/approve/mark_paid must each be audited")
	}
}

func TestEndingAPayoutReleasesTheReservation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	net := e.fund(t, e.a, 1_000_000)
	acct := e.account(t, e.a, e.ua)

	check := func(name string) {
		t.Helper()
		if b := e.bal(t, e.a); b.Available != net || b.Payable != net || b.InPayout != 0 {
			t.Fatalf("%s: funds not released: %+v", name, b)
		}
	}
	p1, _, _ := e.req(e.a, e.ua, acct, 100_000, "key-cancel-0001")
	if c, err := e.svc.Cancel(ctx, e.a, p1.ID, Actor{UserID: e.ua}); err != nil || c.Status != "cancelled" {
		t.Fatalf("cancel: %+v %v", c, err)
	}
	check("cancel")
	if _, err := e.svc.Cancel(ctx, e.b, p1.ID, Actor{UserID: e.ub}); !errors.Is(err, ErrNotFound) {
		// merchant B can never touch A's payout
		t.Fatalf("foreign cancel: %v", err)
	}

	p2, _, _ := e.req(e.a, e.ua, acct, 100_000, "key-reject-0001")
	if _, err := e.svc.Reject(ctx, p2.ID, "no", Actor{UserID: e.fin}); !errors.Is(err, ErrValidation) {
		t.Fatalf("reject without a real note: %v", err)
	}
	if r, err := e.svc.Reject(ctx, p2.ID, "account name mismatch", Actor{UserID: e.fin}); err != nil || r.Status != "cancelled" {
		t.Fatalf("reject: %+v %v", r, err)
	}
	check("reject")

	p3, _, _ := e.req(e.a, e.ua, acct, 100_000, "key-failed-0001")
	e.svc.Approve(ctx, p3.ID, Actor{UserID: e.fin}) //nolint:errcheck
	if f, err := e.svc.MarkFailed(ctx, p3.ID, "bank returned the transfer", Actor{UserID: e.fin}); err != nil || f.Status != "failed" {
		t.Fatalf("mark failed: %+v %v", f, err)
	}
	check("failed")
	if _, err := e.svc.Cancel(ctx, e.a, p3.ID, Actor{UserID: e.ua}); !errors.Is(err, ErrState) {
		t.Fatalf("cancel after failed: %v", err)
	}
	// Each release posted once, and the books still balance.
	if e.count(t, `SELECT count(*) FROM ledger_transactions WHERE source_type = 'payout_released'`) != 3 {
		t.Fatal("expected exactly three release journals")
	}
	e.booksBalance(t)

	// Once released, the same money can be requested again (not stuck).
	if _, _, err := e.req(e.a, e.ua, acct, net, "key-all-in-0001"); err != nil {
		t.Fatalf("re-request after release: %v", err)
	}
}

// Many requests race for money that covers only some of them: the balance is never overspent.
func TestConcurrentRequestsCannotOverspend(t *testing.T) {
	e := setup(t)
	net := e.fund(t, e.a, 1_000_000)
	acct := e.account(t, e.a, e.ua)
	each := net / 3 // exactly three fit
	var ok, insufficient atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := e.req(e.a, e.ua, acct, each, "key-race-"+strconv.Itoa(i)+"-00")
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrInsufficient):
				insufficient.Add(1)
			default:
				t.Errorf("request: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 || insufficient.Load() != 9 {
		t.Fatalf("ok=%d insufficient=%d", ok.Load(), insufficient.Load())
	}
	b := e.bal(t, e.a)
	if b.InPayout != 3*each || b.Available != net-3*each || b.Available < 0 || b.Payable < 0 {
		t.Fatalf("balance after race: %+v", b)
	}
	e.booksBalance(t)
}

func TestRequestIsIdempotent(t *testing.T) {
	e := setup(t)
	net := e.fund(t, e.a, 1_000_000)
	acct := e.account(t, e.a, e.ua)

	first, replayed, err := e.req(e.a, e.ua, acct, 200_000, "key-idem-00001")
	if err != nil || replayed {
		t.Fatalf("first: %v", err)
	}
	again, replayed, err := e.req(e.a, e.ua, acct, 200_000, "key-idem-00001")
	if err != nil || !replayed || again.ID != first.ID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if _, _, err := e.req(e.a, e.ua, acct, 200_001, "key-idem-00001"); !errors.Is(err, idempotency.ErrConflict) {
		t.Fatalf("same key, different amount: %v", err)
	}
	// The same key by ANOTHER merchant is an unrelated operation.
	e.fund(t, e.b, 500_000)
	acctB := e.account(t, e.b, e.ub)
	if _, replayed, err := e.req(e.b, e.ub, acctB, 100_000, "key-idem-00001"); err != nil || replayed {
		t.Fatalf("other merchant, same key: replayed=%v %v", replayed, err)
	}
	if b := e.bal(t, e.a); b.InPayout != 200_000 || b.Available != net-200_000 {
		t.Fatalf("reserved more than once: %+v", b)
	}

	// Retry storm with one key: exactly one payout.
	var wg sync.WaitGroup
	ids := sync.Map{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, _, err := e.req(e.a, e.ua, acct, 50_000, "key-storm-0001")
			if err != nil && !errors.Is(err, idempotency.ErrInProgress) {
				t.Errorf("storm: %v", err)
				return
			}
			if err == nil {
				ids.Store(p.ID, true)
			}
		}()
	}
	wg.Wait()
	n := 0
	ids.Range(func(_, _ any) bool { n++; return true })
	if n != 1 || e.count(t, `SELECT count(*) FROM payouts WHERE merchant_id = $1 AND amount = 50000`, e.a) != 1 {
		t.Fatalf("payouts created by a retry storm: %d", n)
	}
}

func TestOnlySettledFundsAreAvailable(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	acct := e.account(t, e.a, e.ua)
	// A succeeded but NOT settled payment: it is payable, but pending, so not payable-out.
	tx, _ := e.db.Begin(ctx)
	p, _ := e.pay.CreateTx(ctx, tx, payment.CreateInput{MerchantID: e.a, MerchantReference: "U", Amount: 1_000_000, Currency: "IDR", PaymentMethod: "QRIS", Gateway: "gw1"})
	tx.Commit(ctx)                                                                                                        //nolint:errcheck
	e.pay.Transition(ctx, payment.TransitionInput{PaymentID: p.ID, To: payment.Pending, Source: payment.SourceInternal})  //nolint:errcheck
	e.pay.Transition(ctx, payment.TransitionInput{PaymentID: p.ID, To: payment.Succeeded, Source: payment.SourceWebhook}) //nolint:errcheck
	if b := e.bal(t, e.a); b.Payable == 0 || b.Available != 0 {
		t.Fatalf("balance: %+v", b)
	}
	if _, _, err := e.req(e.a, e.ua, acct, 10_000, "key-unsettled-1"); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("payout of unsettled money: %v", err)
	}
}

func TestBankAccountRules(t *testing.T) {
	e := setup(t, func(c *Config) { c.Cooling = time.Hour })
	ctx := context.Background()
	e.fund(t, e.a, 1_000_000)
	e.fund(t, e.b, 1_000_000)

	for _, bad := range [][3]string{{"B", "1234567890", "Name"}, {"BCA", "12ab", "Name"}, {"BCA", "1234567890", "N"}, {"BCA", "12345678901234567890123456789012345", "Name"}} {
		if _, err := e.svc.AddBankAccount(ctx, e.a, bad[0], bad[1], bad[2], Actor{UserID: e.ua}); !errors.Is(err, ErrValidation) {
			t.Errorf("%v accepted: %v", bad, err)
		}
	}
	acct, err := e.svc.AddBankAccount(ctx, e.a, "BCA", "9876543210", "PT A", Actor{UserID: e.ua})
	if err != nil || acct.AccountMasked != "••••3210" || acct.Status != "pending_verification" {
		t.Fatalf("add: %+v %v", acct, err)
	}
	if e.count(t, `SELECT count(*) FROM merchant_bank_accounts WHERE position($1::bytea in account_enc) > 0`, []byte("9876543210")) != 0 {
		t.Fatal("account number stored in plaintext")
	}

	// Unverified, then verified-but-cooling, are not usable. A creator cannot verify their own account.
	if _, _, err := e.req(e.a, e.ua, acct.ID, 10_000, "key-acct-0001"); !errors.Is(err, ErrAccountUnusable) {
		t.Fatalf("unverified: %v", err)
	}
	if _, err := e.svc.VerifyBankAccount(ctx, acct.ID, Actor{UserID: e.ua}); !errors.Is(err, ErrState) {
		t.Fatalf("self verification: %v", err)
	}
	if _, err := e.svc.VerifyBankAccount(ctx, acct.ID, Actor{UserID: e.fin}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.req(e.a, e.ua, acct.ID, 10_000, "key-acct-0002"); !errors.Is(err, ErrAccountUnusable) {
		t.Fatalf("cooling period ignored: %v", err)
	}
	if _, err := e.db.Exec(ctx, `UPDATE merchant_bank_accounts SET verified_at = now() - interval '2 hours' WHERE id = $1`, acct.ID); err != nil {
		t.Fatal(err)
	}
	p, _, err := e.req(e.a, e.ua, acct.ID, 10_000, "key-acct-0003")
	if err != nil {
		t.Fatalf("after cooling: %v", err)
	}

	// Another merchant's account is never usable; nor garbage ids.
	other := e.account(t, e.b, e.ub)
	e.db.Exec(ctx, `UPDATE merchant_bank_accounts SET verified_at = now() - interval '2 hours'`) //nolint:errcheck
	for _, id := range []string{other, "not-a-uuid", "99999999-9999-4999-8999-999999999999"} {
		if _, _, err := e.req(e.a, e.ua, id, 10_000, "key-foreign-"+id[:4]); !errors.Is(err, ErrAccountUnusable) {
			t.Errorf("account %q: %v", id, err)
		}
	}
	// An account with an open payout cannot be retired from under it; a tenant cannot retire another's.
	if err := e.svc.DisableBankAccount(ctx, e.a, acct.ID, Actor{UserID: e.ua}); !errors.Is(err, ErrAccountInUse) {
		t.Fatalf("disable with open payout: %v", err)
	}
	if err := e.svc.DisableBankAccount(ctx, e.b, acct.ID, Actor{UserID: e.ub}); !errors.Is(err, ErrAccountInUse) && !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign disable: %v", err)
	}
	e.svc.Cancel(ctx, e.a, p.ID, Actor{UserID: e.ua}) //nolint:errcheck
	if err := e.svc.DisableBankAccount(ctx, e.a, acct.ID, Actor{UserID: e.ua}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.req(e.a, e.ua, acct.ID, 10_000, "key-acct-0004"); !errors.Is(err, ErrAccountUnusable) {
		t.Fatalf("disabled account: %v", err)
	}
}

func TestLimitsAndEnvironmentGate(t *testing.T) {
	e := setup(t, func(c *Config) { c.MinAmount, c.MaxAmount, c.DailyLimit = 50_000, 300_000, 400_000 })
	e.fund(t, e.a, 5_000_000)
	acct := e.account(t, e.a, e.ua)
	for i, tc := range []struct {
		amount int64
		want   error
	}{{49_999, ErrLimit}, {300_001, ErrLimit}, {0, ErrValidation}, {-5, ErrValidation}, {300_000, nil}, {100_001, ErrLimit}, {100_000, nil}} {
		if _, _, err := e.req(e.a, e.ua, acct, tc.amount, fmt.Sprintf("key-limit-%04d", i)); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("amount %d: got %v want %v", tc.amount, err, tc.want)
		}
	}
	// Rejected requests reserved nothing.
	if b := e.bal(t, e.a); b.InPayout != 400_000 {
		t.Fatalf("in payout: %+v", b)
	}

	// Production without the go/no-go switch: refused before anything is touched.
	prod := setup(t, func(c *Config) { c.DryRun, c.LiveEnabled = false, false })
	prod.fund(t, prod.a, 1_000_000)
	pacct := prod.account(t, prod.a, prod.ua)
	if _, _, err := prod.req(prod.a, prod.ua, pacct, 100_000, "key-prod-00001"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("production payout without ERAPAY_PAYOUT_LIVE: %v", err)
	}
	if prod.count(t, `SELECT count(*) FROM payouts`) != 0 || prod.bal(t, prod.a).InPayout != 0 {
		t.Fatal("a disabled payout touched the books")
	}
	live := setup(t, func(c *Config) { c.DryRun, c.LiveEnabled = false, true })
	live.fund(t, live.a, 1_000_000)
	p, _, err := live.req(live.a, live.ua, live.account(t, live.a, live.ua), 100_000, "key-live-000001")
	if err != nil || p.DryRun {
		t.Fatalf("live payout: %+v %v", p, err)
	}
}

func TestDatabaseGuardsPayouts(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.fund(t, e.a, 1_000_000)
	acct := e.account(t, e.a, e.ua)
	p, _, _ := e.req(e.a, e.ua, acct, 100_000, "key-guard-0001")
	acct2 := e.account(t, e.a, e.ua) // a different, verified destination

	for name, q := range map[string]string{
		"maker approves own payout": fmt.Sprintf(`UPDATE payouts SET status='approved', approved_by=requested_by, approved_at=now() WHERE id='%s'`, p.ID),
		"approved without checker":  fmt.Sprintf(`UPDATE payouts SET status='approved' WHERE id='%s'`, p.ID),
		"paid without reference":    fmt.Sprintf(`UPDATE payouts SET status='paid', paid_at=now() WHERE id='%s'`, p.ID),
		"change amount":             fmt.Sprintf(`UPDATE payouts SET amount = amount + 1 WHERE id='%s'`, p.ID),
		"change destination":        fmt.Sprintf(`UPDATE payouts SET bank_account_id = '%s' WHERE id='%s'`, acct2, p.ID),
		"delete":                    fmt.Sprintf(`DELETE FROM payouts WHERE id='%s'`, p.ID),
		"zero amount insert":        fmt.Sprintf(`INSERT INTO payouts (merchant_id, bank_account_id, amount, currency, dry_run, requested_by) VALUES ('%s','%s',0,'IDR',true,'%s')`, e.a, acct, e.ua),
	} {
		if _, err := e.db.Exec(ctx, q); err == nil {
			t.Errorf("%s: database allowed it", name)
		}
	}
	// Final payouts cannot be revived.
	e.svc.Approve(ctx, p.ID, Actor{UserID: e.fin})                //nolint:errcheck
	e.svc.MarkPaid(ctx, p.ID, "BANK-REF-9", Actor{UserID: e.fin}) //nolint:errcheck
	if _, err := e.db.Exec(ctx, `UPDATE payouts SET status='approved' WHERE id=$1`, p.ID); err == nil {
		t.Error("a paid payout was moved back")
	}
}

func TestRevealDestinationIsApprovedOnlyAndAudited(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.fund(t, e.a, 1_000_000)
	acct := e.account(t, e.a, e.ua)
	p, _, _ := e.req(e.a, e.ua, acct, 100_000, "key-reveal-001")
	if _, err := e.svc.RevealDestination(ctx, p.ID, Actor{UserID: e.fin}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reveal before approval: %v", err)
	}
	e.svc.Approve(ctx, p.ID, Actor{UserID: e.fin}) //nolint:errcheck
	d, err := e.svc.RevealDestination(ctx, p.ID, Actor{UserID: e.fin})
	if err != nil || d.AccountNumber != "1234567890" || d.BankCode != "BCA" || d.AccountName != "PT Merchant" {
		t.Fatalf("reveal: %+v %v", d, err)
	}
	if e.count(t, `SELECT count(*) FROM audit_logs WHERE action = 'payout.reveal_destination'`) != 1 {
		t.Fatal("reveal not audited")
	}
	if e.count(t, `SELECT count(*) FROM audit_logs WHERE metadata::text LIKE '%1234567890%'`) != 0 {
		t.Fatal("full account number leaked into the audit log")
	}
	e.svc.MarkFailed(ctx, p.ID, "cancelled by ops", Actor{UserID: e.fin}) //nolint:errcheck
	if _, err := e.svc.RevealDestination(ctx, p.ID, Actor{UserID: e.fin}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reveal after the payout ended: %v", err)
	}
}
