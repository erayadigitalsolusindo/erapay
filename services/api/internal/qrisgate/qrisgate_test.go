package qrisgate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/payment"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/platformsetting"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type env struct {
	db                    *pgxpool.Pool
	svc                   *Service
	set                   *platformsetting.Service
	pay                   *payment.Service
	merchant, tenant, fin string
}

func setup(t *testing.T) env {
	t.Helper()
	db := testdb.New(t)
	m, _ := testdb.SeedMerchant(t, db, "QG")
	user := func(email string) string {
		var id string
		if err := db.QueryRow(context.Background(), `INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id::text`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	var plan string
	if err := db.QueryRow(context.Background(), `
		INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
		VALUES ('std-gw1', 1, 'gw1', 'QRIS', 70, 'half_up', 100, 'half_up') RETURNING id::text`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(context.Background(), `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
		VALUES ($1,$2, now() - interval '1 day')`, m, plan); err != nil {
		t.Fatal(err)
	}
	set := platformsetting.NewService(db)
	pay := payment.NewService(db)
	return env{db, NewService(db, set, &fakePayer{db: db, svc: pay}), set, pay, m, user("tenant@x.test"), user("fin@x.test")}
}

// fakePayer stands in for the orchestrator: it creates the payment row the way it does (same purpose rules),
// remembers it per Idempotency-Key, and leaves it pending like a gateway that accepted the QR.
type fakePayer struct {
	db    *pgxpool.Pool
	svc   *payment.Service
	mu    sync.Mutex
	byKey map[string]payment.Payment
}

func (f *fakePayer) Create(ctx context.Context, r payment.CreateRequest) (payment.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.byKey[r.IdempotencyKey]; ok {
		return payment.Outcome{Payment: p, Replayed: true}, nil
	}
	tx, _ := f.db.Begin(ctx)
	defer tx.Rollback(ctx) //nolint:errcheck
	p, err := f.svc.CreateTx(ctx, tx, payment.CreateInput{MerchantID: r.MerchantID, MerchantReference: r.MerchantReference, Description: r.Description,
		Amount: r.Amount, Currency: r.Currency, PaymentMethod: r.PaymentMethod, Gateway: "gw1", Purpose: r.Purpose})
	if err != nil {
		return payment.Outcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return payment.Outcome{}, err
	}
	if _, err := f.svc.Transition(ctx, payment.TransitionInput{PaymentID: p.ID, To: payment.Pending, Source: payment.SourceInternal, QRPayload: "00020101QR"}); err != nil {
		return payment.Outcome{}, err
	}
	if f.byKey == nil {
		f.byKey = map[string]payment.Payment{}
	}
	f.byKey[r.IdempotencyKey] = p
	return payment.Outcome{Payment: p}, nil
}

func (e env) req(t *testing.T, key string, amount int64) Topup {
	t.Helper()
	tp, err := e.svc.RequestTopup(context.Background(), e.merchant, e.tenant, key, amount)
	if err != nil {
		t.Fatal(err)
	}
	return tp
}

func (e env) charge(t *testing.T, payment string) int64 {
	t.Helper()
	tx, _ := e.db.Begin(context.Background())
	defer tx.Rollback(context.Background()) //nolint:errcheck
	fee, err := e.svc.ChargeFee(context.Background(), tx, e.merchant, payment)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	return fee
}

func (e env) bal(t *testing.T) int64 {
	t.Helper()
	b, err := e.svc.Balance(context.Background(), e.merchant)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRequestIsIdempotentAndRejectsDifferentAmount(t *testing.T) {
	e := setup(t)
	a := e.req(t, "key-00000001", 50_000)
	b := e.req(t, "key-00000001", 50_000)
	if a.ID != b.ID || !b.Replayed {
		t.Fatalf("replay should return the first request: %+v %+v", a, b)
	}
	if _, err := e.svc.RequestTopup(context.Background(), e.merchant, e.tenant, "key-00000001", 60_000); !errors.Is(err, ErrConflict) {
		t.Fatalf("same key, other amount: %v", err)
	}
	if _, err := e.svc.RequestTopup(context.Background(), e.merchant, e.tenant, "key-00000002", 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero amount: %v", err)
	}
	if e.bal(t) != 0 {
		t.Fatal("a request alone must not change the balance")
	}
}

func TestConcurrentConfirmCreditsOnce(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tp := e.req(t, "key-00000003", 50_000)
	if _, err := e.svc.Decide(ctx, tp.ID, e.tenant, "", true, "TRF-1", ""); !errors.Is(err, ErrSelfDecision) {
		t.Fatalf("requester confirming own top-up: %v", err)
	}
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Decide(ctx, tp.ID, e.fin, "", true, "TRF-1", ""); err == nil {
				ok.Add(1)
			} else if !errors.Is(err, ErrInvalidState) {
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || e.bal(t) != 50_000 {
		t.Fatalf("confirmed %d times, balance %d; want 1 and 50000", ok.Load(), e.bal(t))
	}
	if _, err := e.svc.Decide(ctx, tp.ID, e.fin, "", false, "", "tolak"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("reject after confirm: %v", err)
	}
}

func TestRejectedAndCancelledDoNotCredit(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a, b := e.req(t, "key-00000004", 10_000), e.req(t, "key-00000005", 20_000)
	if _, err := e.svc.Decide(ctx, a.ID, e.fin, "", false, "", "transfer tidak masuk"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.CancelTopup(ctx, e.merchant, b.ID); err != nil {
		t.Fatal(err)
	}
	if e.bal(t) != 0 {
		t.Fatalf("balance %d; want 0", e.bal(t))
	}
}

func TestFeeIsOffByDefaultThenChargesOncePerPaymentAndMayGoNegative(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if fee := e.charge(t, "pay-1"); fee != 0 || e.bal(t) != 0 {
		t.Fatalf("default fee must be a no-op: fee %d balance %d", fee, e.bal(t))
	}
	if err := e.set.SetFee(ctx, 100, "biaya awal", "", ""); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ { // same payment charged concurrently and again later: once
		wg.Add(1)
		go func() { defer wg.Done(); e.charge(t, "pay-2") }()
	}
	wg.Wait()
	e.charge(t, "pay-2")
	if e.bal(t) != -100 {
		t.Fatalf("balance %d; want -100 (negative allowed, charged once)", e.bal(t))
	}
	e.charge(t, "pay-3")
	tp := e.req(t, "key-00000006", 1_000)
	if _, err := e.svc.Decide(ctx, tp.ID, e.fin, "", true, "TRF-2", ""); err != nil {
		t.Fatal(err)
	}
	if e.bal(t) != 800 {
		t.Fatalf("balance %d; want 800", e.bal(t))
	}
	h, err := e.svc.History(ctx, e.merchant, 10)
	if err != nil || len(h) != 3 {
		t.Fatalf("history %+v %v; want 3 movements", h, err)
	}
	var revenue int64
	if err := e.db.QueryRow(ctx, `SELECT COALESCE(-SUM(debit_minus_credit),0) FROM ledger_account_balances WHERE code = 'platform_fee_revenue'`).Scan(&revenue); err != nil || revenue != 200 {
		t.Fatalf("platform revenue %d %v; want 200", revenue, err)
	}
}

func (e env) count(t *testing.T, q string, args ...any) (n int64) {
	t.Helper()
	if err := e.db.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestQRISTopupCreditsNetOnceAndStaysOutOfMerchantSales(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tp, err := e.svc.RequestQRISTopup(ctx, e.merchant, e.tenant, "key-qris0001", 100_000)
	if err != nil || tp.PaymentID == "" || tp.Method != "qris" || tp.QRPayload == "" {
		t.Fatalf("request: %+v %v", tp, err)
	}
	again, err := e.svc.RequestQRISTopup(ctx, e.merchant, e.tenant, "key-qris0001", 100_000)
	if err != nil || again.ID != tp.ID || again.PaymentID != tp.PaymentID || !again.Replayed {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if _, err := e.svc.RequestQRISTopup(ctx, e.merchant, e.tenant, "key-qris0001", 200_000); !errors.Is(err, ErrConflict) {
		t.Fatalf("same key, other amount: %v", err)
	}
	if _, err := e.svc.RequestQRISTopup(ctx, e.merchant, e.tenant, "key-qris0002", MinQRISTopup-1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("below minimum: %v", err)
	}
	if n := e.count(t, `SELECT count(*) FROM payments WHERE purpose = 'qrisgate_topup'`); n != 1 {
		t.Fatalf("%d top-up payments; want 1", n)
	}
	// Tenant bears the MDR (0.7% = 700), no platform fee: net 99.300 is what gets credited.
	if n := e.count(t, `SELECT merchant_net FROM payments WHERE id = $1`, tp.PaymentID); n != 99_300 {
		t.Fatalf("merchant_net %d; want 99300", n)
	}
	if _, err := e.svc.Decide(ctx, tp.ID, e.fin, "", true, "TRF-X", ""); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("hand-confirming a QRIS top-up: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ { // webhook and inquiry racing for the same payment
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.pay.Transition(ctx, payment.TransitionInput{PaymentID: tp.PaymentID, To: payment.Succeeded, Source: payment.SourceWebhook}); err != nil {
				t.Errorf("transition: %v", err)
			}
		}()
	}
	wg.Wait()

	if e.bal(t) != 99_300 {
		t.Fatalf("balance %d; want 99300 credited once", e.bal(t))
	}
	if got, _ := e.svc.ListTopups(ctx, e.merchant, "confirmed", 10); len(got) != 1 {
		t.Fatalf("confirmed top-ups %d; want 1", len(got))
	}
	if n := e.count(t, `SELECT COALESCE(SUM(debit_minus_credit),0) FROM ledger_account_balances WHERE code = 'merchant_payable'`); n != 0 {
		t.Fatalf("merchant_payable %d; a top-up must not touch the merchant's sales balance", n)
	}
	if n := e.count(t, `SELECT COALESCE(SUM(debit_minus_credit),0) FROM ledger_account_balances WHERE code = 'platform_fee_revenue'`); n != 0 {
		t.Fatalf("platform revenue %d; a top-up charges no platform fee", n)
	}
	if n := e.count(t, `SELECT count(*) FROM webhook_events WHERE merchant_id = $1`, e.merchant); n != 0 {
		t.Fatalf("%d merchant webhook events queued for a top-up", n)
	}
	if ps, _, err := e.pay.List(ctx, payment.ListFilter{MerchantID: e.merchant}); err != nil || len(ps) != 0 {
		t.Fatalf("top-up payment leaked into the merchant's transactions: %v %v", ps, err)
	}
}

func TestFailedQRISTopupClosesRequestWithoutCredit(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tp, err := e.svc.RequestQRISTopup(ctx, e.merchant, e.tenant, "key-qris0003", 50_000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pay.Transition(ctx, payment.TransitionInput{PaymentID: tp.PaymentID, To: payment.Expired, Source: payment.SourceInquiry}); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.ListTopups(ctx, e.merchant, "cancelled", 10); len(got) != 1 || e.bal(t) != 0 {
		t.Fatalf("cancelled %d balance %d; want 1 and 0", len(got), e.bal(t))
	}
}
