package ledger

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/pricing"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

var plan = pricing.Plan{Version: 1,
	MDR:      pricing.Rate{BPS: 70, Rounding: pricing.RoundHalfUp},
	Platform: pricing.Rate{BPS: 100, Rounding: pricing.RoundHalfUp}}

func merchant(t *testing.T, db *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(),
		`INSERT INTO merchants (legal_name, display_name) VALUES ($1,$1) RETURNING id::text`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func breakdown(t *testing.T, gross int64) pricing.Breakdown {
	t.Helper()
	b, err := pricing.Compute(gross, "IDR", plan)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func post(t *testing.T, s *Service, mid, pid string, gross int64) Result {
	t.Helper()
	tx, err := PaymentSucceeded(mid, pid, breakdown(t, gross))
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Post(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Books must always balance globally, and balances must equal the entry sums.
func assertBooks(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	var d, c int64
	if err := db.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(amount) FILTER (WHERE direction='debit'),0),
		       COALESCE(SUM(amount) FILTER (WHERE direction='credit'),0) FROM ledger_entries`).Scan(&d, &c); err != nil {
		t.Fatal(err)
	}
	if d != c {
		t.Fatalf("books unbalanced: debit %d credit %d", d, c)
	}
}

func account(t *testing.T, db *pgxpool.Pool, code string) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(debit_minus_credit),0) FROM ledger_account_balances WHERE code=$1`, code).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPaymentSucceededPosting(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	mid := merchant(t, db, "A")

	r := post(t, s, mid, "pay-1", 1000) // MDR 7, fee 10, net 983
	if r.Duplicate {
		t.Fatal("first post flagged duplicate")
	}
	bal, _ := s.MerchantBalance(context.Background(), mid, "IDR")
	if bal.Payable != 983 {
		t.Fatalf("payable = %d, want 983", bal.Payable)
	}
	if got := account(t, db, CashDokuClearing); got != 993 { // gross - MDR expected from gateway
		t.Fatalf("clearing = %d, want 993", got)
	}
	if got := account(t, db, PlatformFeeRevenue); got != -10 {
		t.Fatalf("revenue (credit) = %d, want -10", got)
	}
	assertBooks(t, db)
}

func TestDuplicateEventPostsOnce(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	mid := merchant(t, db, "A")
	r1 := post(t, s, mid, "pay-1", 1000)
	r2 := post(t, s, mid, "pay-1", 1000)
	if !r2.Duplicate || r1.TxnID != r2.TxnID {
		t.Fatalf("r1=%+v r2=%+v", r1, r2)
	}
	var n int
	_ = db.QueryRow(context.Background(), `SELECT count(*) FROM ledger_entries`).Scan(&n)
	if n != 3 {
		t.Fatalf("entries = %d, want 3", n)
	}
	// Same key, different event content -> refused, not silently deduped.
	other, _ := PaymentSucceeded(mid, "pay-1", breakdown(t, 2000))
	other.SourceID = "other"
	if _, err := s.Post(context.Background(), other); !errors.Is(err, ErrDedupMismatch) {
		t.Fatalf("want ErrDedupMismatch, got %v", err)
	}
}

// Same event delivered 40x in parallel -> exactly one journal.
func TestConcurrentSameEvent(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	mid := merchant(t, db, "A")
	var wg sync.WaitGroup
	var fresh, dup atomic.Int32
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, _ := PaymentSucceeded(mid, "pay-race", breakdown(t, 100000))
			r, err := s.Post(context.Background(), tx)
			if err != nil {
				t.Errorf("post: %v", err)
				return
			}
			if r.Duplicate {
				dup.Add(1)
			} else {
				fresh.Add(1)
			}
		}()
	}
	wg.Wait()
	if fresh.Load() != 1 || dup.Load() != 39 {
		t.Fatalf("fresh=%d dup=%d", fresh.Load(), dup.Load())
	}
	bal, _ := s.MerchantBalance(context.Background(), mid, "IDR")
	if want := breakdown(t, 100000).MerchantNet; bal.Payable != want {
		t.Fatalf("payable = %d, want %d", bal.Payable, want)
	}
	assertBooks(t, db)
}

// Many different payments in parallel for two merchants: balances equal the exact sums.
func TestConcurrentDistinctPayments(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	a, b := merchant(t, db, "A"), merchant(t, db, "B")
	rng := rand.New(rand.NewSource(7))
	var wantA, wantB, wantRev int64
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		gross := int64(rng.Intn(5_000_000) + 1)
		mid := a
		bd := breakdown(t, gross)
		wantRev += bd.PlatformFee
		if i%3 == 0 {
			mid = b
			wantB += bd.MerchantNet
		} else {
			wantA += bd.MerchantNet
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, _ := PaymentSucceeded(mid, "pay-"+strconv.Itoa(i), bd)
			if _, err := s.Post(context.Background(), tx); err != nil {
				t.Errorf("post %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	ba, _ := s.MerchantBalance(context.Background(), a, "IDR")
	bb, _ := s.MerchantBalance(context.Background(), b, "IDR")
	if ba.Payable != wantA || bb.Payable != wantB {
		t.Fatalf("A=%d/%d B=%d/%d", ba.Payable, wantA, bb.Payable, wantB)
	}
	if got := -account(t, db, PlatformFeeRevenue); got != wantRev {
		t.Fatalf("revenue = %d, want %d", got, wantRev)
	}
	var accounts int
	_ = db.QueryRow(context.Background(), `SELECT count(*) FROM ledger_accounts`).Scan(&accounts)
	if accounts != 4 { // clearing, revenue, payable(A), payable(B): no duplicates under races
		t.Fatalf("accounts = %d, want 4", accounts)
	}
	assertBooks(t, db)
}

func TestReverseOnceAndNets(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	mid := merchant(t, db, "A")
	post(t, s, mid, "pay-1", 1000)

	var wg sync.WaitGroup
	var fresh atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Reverse(context.Background(), "payment_succeeded:pay-1", "test")
			if err != nil {
				t.Errorf("reverse: %v", err)
			} else if !r.Duplicate {
				fresh.Add(1)
			}
		}()
	}
	wg.Wait()
	if fresh.Load() != 1 {
		t.Fatalf("reversal posted %d times", fresh.Load())
	}
	bal, _ := s.MerchantBalance(context.Background(), mid, "IDR")
	if bal.Payable != 0 || account(t, db, CashDokuClearing) != 0 || account(t, db, PlatformFeeRevenue) != 0 {
		t.Fatalf("reversal did not net to zero: %+v", bal)
	}
	if _, err := s.Reverse(context.Background(), "payment_succeeded:nope", "x"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	assertBooks(t, db)
}

func TestInvalidJournalsRejected(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	mid := merchant(t, db, "A")
	ctx := context.Background()
	bad := []Txn{
		{DedupKey: "k1", SourceType: "x", SourceID: "1", Currency: "IDR", Entries: []Entry{
			{CashDokuClearing, "", Debit, 100}, {PlatformFeeRevenue, "", Credit, 90}}}, // unbalanced
		{DedupKey: "k2", SourceType: "x", SourceID: "1", Currency: "IDR", Entries: []Entry{
			{CashDokuClearing, "", Debit, 0}, {PlatformFeeRevenue, "", Credit, 0}}}, // zero amounts
		{DedupKey: "k3", SourceType: "x", SourceID: "1", Currency: "IDR", Entries: []Entry{
			{CashDokuClearing, "", Debit, 5}, {MerchantPayable, "", Credit, 5}}}, // payable without merchant
		{DedupKey: "k4", SourceType: "x", SourceID: "1", Currency: "IDR", Entries: []Entry{
			{CashDokuClearing, mid, Debit, 5}, {PlatformFeeRevenue, "", Credit, 5}}}, // platform acct with merchant
	}
	for i, tx := range bad {
		if _, err := s.Post(ctx, tx); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	var n int
	_ = db.QueryRow(ctx, `SELECT count(*) FROM ledger_transactions`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d transactions leaked from rejected posts", n)
	}
}

// Defense in depth: even bypassing Go validation, the DB refuses unbalanced journals
// and any mutation of posted ones.
func TestDatabaseEnforcesInvariants(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	mid := merchant(t, db, "A")
	ctx := context.Background()
	post(t, s, mid, "pay-1", 1000)

	tx, _ := db.Begin(ctx)
	var tid, acc string
	_ = tx.QueryRow(ctx, `INSERT INTO ledger_transactions (dedup_key, source_type, source_id, currency)
		VALUES ('raw','x','1','IDR') RETURNING id::text`).Scan(&tid)
	_ = tx.QueryRow(ctx, `SELECT id::text FROM ledger_accounts LIMIT 1`).Scan(&acc)
	_, _ = tx.Exec(ctx, `INSERT INTO ledger_entries (txn_id, account_id, direction, amount) VALUES ($1,$2,'debit',10)`, tid, acc)
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("unbalanced journal committed")
	}
	for _, q := range []string{
		`UPDATE ledger_entries SET amount = amount + 1`,
		`DELETE FROM ledger_entries`,
		`UPDATE ledger_transactions SET source_id = 'z'`,
		`DELETE FROM ledger_transactions`,
	} {
		if _, err := db.Exec(ctx, q); err == nil {
			t.Fatalf("mutation allowed: %s", q)
		}
	}
	assertBooks(t, db)
}

// Property: for any amount, posted balances match the pricing breakdown exactly.
func TestPostingMatchesBreakdownProperty(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	mid := merchant(t, db, "A")
	rng := rand.New(rand.NewSource(42))
	var net, rev, clr int64
	for i := 0; i < 200; i++ {
		gross := int64(rng.Intn(10_000_000) + 1)
		if i < 5 {
			gross = int64(i + 1) // tiny amounts where fees round to 0
		}
		bd := breakdown(t, gross)
		net += bd.MerchantNet
		rev += bd.PlatformFee
		clr += bd.Gross - bd.MDRFee
		post(t, s, mid, "p"+strconv.Itoa(i), gross)
	}
	bal, _ := s.MerchantBalance(context.Background(), mid, "IDR")
	if bal.Payable != net || -account(t, db, PlatformFeeRevenue) != rev || account(t, db, CashDokuClearing) != clr {
		t.Fatalf("payable %d/%d rev %d/%d clearing %d/%d", bal.Payable, net, -account(t, db, PlatformFeeRevenue), rev, account(t, db, CashDokuClearing), clr)
	}
	assertBooks(t, db)
}
