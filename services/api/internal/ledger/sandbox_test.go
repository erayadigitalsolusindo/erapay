package ledger

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

// sandboxTwin creates a sandbox merchant owned by a new production merchant; returns (liveID, sandboxID).
func sandboxTwin(t *testing.T, db *pgxpool.Pool, name string) (string, string) {
	t.Helper()
	live := merchant(t, db, name)
	var sbx string
	if err := db.QueryRow(context.Background(),
		`INSERT INTO merchants (legal_name, display_name, is_sandbox, parent_id) VALUES ($1,$1,true,$2) RETURNING id::text`,
		name+" sandbox", live).Scan(&sbx); err != nil {
		t.Fatal(err)
	}
	return live, sbx
}

func platformFee(t *testing.T, db *pgxpool.Pool, livemode bool) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(-debit_minus_credit),0) FROM ledger_account_balances WHERE code='platform_fee_revenue' AND livemode=$1`,
		livemode).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// Sandbox money lands in sandbox platform accounts only; real platform accounts never move.
func TestSandboxPostsToSandboxWorld(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	live, sbx := sandboxTwin(t, db, "A")
	post(t, s, sbx, "pay-sbx", 100000)
	if got := platformFee(t, db, true); got != 0 {
		t.Fatalf("real platform fee moved by sandbox payment: %d", got)
	}
	if got, want := platformFee(t, db, false), breakdown(t, 100000).PlatformFee; got != want {
		t.Fatalf("sandbox fee = %d, want %d", got, want)
	}
	post(t, s, live, "pay-live", 100000)
	if got, want := platformFee(t, db, true), breakdown(t, 100000).PlatformFee; got != want {
		t.Fatalf("real fee = %d, want %d", got, want)
	}
	var n int
	_ = db.QueryRow(context.Background(), `SELECT count(*) FROM ledger_accounts WHERE code='platform_fee_revenue'`).Scan(&n)
	if n != 2 {
		t.Fatalf("platform_fee_revenue accounts = %d, want 2 (one per world)", n)
	}
	assertBooks(t, db)
}

// A reversal of a sandbox journal stays in the sandbox world and nets to zero.
func TestSandboxReverseStaysInWorld(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	_, sbx := sandboxTwin(t, db, "A")
	post(t, s, sbx, "pay-1", 50000)
	if _, err := s.Reverse(context.Background(), "payment_succeeded:pay-1", "test"); err != nil {
		t.Fatal(err)
	}
	if platformFee(t, db, true) != 0 || platformFee(t, db, false) != 0 {
		t.Fatal("reversal did not net to zero in its own world")
	}
}

// One journal cannot span both worlds, in code and in the database.
func TestMixedWorldsRejected(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	live, sbx := sandboxTwin(t, db, "A")
	mixed := Txn{DedupKey: "mix", SourceType: "x", SourceID: "1", Currency: "IDR", Entries: []Entry{
		{MerchantPayable, live, Credit, 100}, {MerchantPayable, sbx, Debit, 100}}}
	if _, err := s.Post(context.Background(), mixed); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}

	// Bypass the Go check: raw entries on a live and a sandbox account must fail at commit.
	ctx := context.Background()
	post(t, s, live, "p-live", 1000)
	post(t, s, sbx, "p-sbx", 1000)
	tx, _ := db.Begin(ctx)
	var tid, la, sa string
	_ = tx.QueryRow(ctx, `INSERT INTO ledger_transactions (dedup_key, source_type, source_id, currency)
		VALUES ('raw','x','1','IDR') RETURNING id::text`).Scan(&tid)
	_ = tx.QueryRow(ctx, `SELECT id::text FROM ledger_accounts WHERE code='platform_fee_revenue' AND livemode`).Scan(&la)
	_ = tx.QueryRow(ctx, `SELECT id::text FROM ledger_accounts WHERE code='platform_fee_revenue' AND NOT livemode`).Scan(&sa)
	_, _ = tx.Exec(ctx, `INSERT INTO ledger_entries (txn_id, account_id, direction, amount) VALUES ($1,$2,'debit',10),($1,$3,'credit',10)`, tid, la, sa)
	if err := tx.Commit(ctx); err == nil {
		t.Fatal("journal mixing sandbox and live accounts committed")
	}
	// A merchant account must live in its merchant's world.
	if _, err := db.Exec(ctx, `INSERT INTO ledger_accounts (code, merchant_id, currency, livemode)
		VALUES ('merchant_payable',$1,'IDR',true)`, sbx); err == nil {
		t.Fatal("live account created for a sandbox merchant")
	}
	assertBooks(t, db)
}

// Same sandbox event in parallel -> one journal and one sandbox account per code.
func TestSandboxConcurrentSameEvent(t *testing.T) {
	db := testdb.New(t)
	s := NewService(db)
	_, sbx := sandboxTwin(t, db, "A")
	var wg sync.WaitGroup
	var fresh atomic.Int32
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, _ := PaymentSucceeded(sbx, "pay-race", breakdown(t, 100000))
			r, err := s.Post(context.Background(), tx)
			if err != nil {
				t.Errorf("post: %v", err)
			} else if !r.Duplicate {
				fresh.Add(1)
			}
		}()
	}
	wg.Wait()
	if fresh.Load() != 1 {
		t.Fatalf("fresh = %d, want 1", fresh.Load())
	}
	if got, want := platformFee(t, db, false), breakdown(t, 100000).PlatformFee; got != want {
		t.Fatalf("sandbox fee = %d, want %d", got, want)
	}
	assertBooks(t, db)
}

// A merchant has at most one sandbox twin, and is_sandbox always has a parent.
func TestSandboxTwinConstraints(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	live, _ := sandboxTwin(t, db, "A")
	if _, err := db.Exec(ctx,
		`INSERT INTO merchants (legal_name, display_name, is_sandbox, parent_id) VALUES ('x','x',true,$1)`, live); err == nil {
		t.Fatal("second sandbox twin allowed")
	}
	if _, err := db.Exec(ctx, `INSERT INTO merchants (legal_name, display_name, is_sandbox) VALUES ('y','y',true)`); err == nil {
		t.Fatal("sandbox merchant without parent allowed")
	}
}
