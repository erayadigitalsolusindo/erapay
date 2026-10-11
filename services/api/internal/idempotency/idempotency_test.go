package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type fixture struct {
	db  *pgxpool.Pool
	st  *Store
	mid string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testdb.New(t)
	mid, _ := testdb.SeedMerchant(t, db, "A")
	if _, err := db.Exec(context.Background(), `CREATE TABLE effects (id serial PRIMARY KEY, note text)`); err != nil {
		t.Fatal(err)
	}
	return fixture{db: db, st: NewStore(db), mid: mid}
}

func (f fixture) effects(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(context.Background(), `SELECT count(*) FROM effects`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// domain side effect: one row in `effects`, returned as the resource.
func effect(ctx context.Context, tx pgx.Tx) (Response, error) {
	var id int
	if err := tx.QueryRow(ctx, `INSERT INTO effects (note) VALUES ('x') RETURNING id`).Scan(&id); err != nil {
		return Response{}, err
	}
	b, _ := json.Marshal(map[string]int{"id": id})
	return Response{Status: 201, Body: b, ResourceType: "effect", ResourceID: strconv.Itoa(id)}, nil
}

func hash(t *testing.T, body string) []byte {
	t.Helper()
	h, err := HashJSON([]byte(body), "POST", "/v1/x")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestHashJSONCanonical(t *testing.T) {
	a := hash(t, `{"amount": 10000, "currency":"IDR"}`)
	b := hash(t, `{ "currency":"IDR",   "amount":10000 }`)
	c := hash(t, `{"amount":10001,"currency":"IDR"}`)
	if string(a) != string(b) {
		t.Fatal("key order/whitespace must not change the hash")
	}
	if string(a) == string(c) {
		t.Fatal("different amount must change the hash")
	}
	if ValidKey("short") || ValidKey("has space in it") || !ValidKey("order-2026-0001") {
		t.Fatal("key validation")
	}
}

func TestDoDuplicateReplaysFirstResult(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := Scope{f.mid, "payment.create", "key-0000001"}
	h := hash(t, `{"amount":10000}`)

	r1, replayed, err := f.st.Do(ctx, sc, h, effect)
	if err != nil || replayed {
		t.Fatalf("first: replayed=%v err=%v", replayed, err)
	}
	r2, replayed, err := f.st.Do(ctx, sc, h, effect)
	if err != nil || !replayed {
		t.Fatalf("second: replayed=%v err=%v", replayed, err)
	}
	if r1.ResourceID != r2.ResourceID || r2.Status != 201 || string(r1.Body) != string(r2.Body) {
		t.Fatalf("replay differs: %+v vs %+v", r1, r2)
	}
	if n := f.effects(t); n != 1 {
		t.Fatalf("side effects = %d, want 1", n)
	}

	// Same key, different payload -> conflict, no side effect.
	_, _, err = f.st.Do(ctx, sc, hash(t, `{"amount":20000}`), effect)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	// Same key on another operation or another merchant is a different scope.
	other, _ := testdb.SeedMerchant(t, f.db, "B")
	for _, s := range []Scope{{f.mid, "payout.create", sc.Key}, {other, "payment.create", sc.Key}} {
		if _, replayed, err := f.st.Do(ctx, s, h, effect); err != nil || replayed {
			t.Fatalf("scope %+v: replayed=%v err=%v", s, replayed, err)
		}
	}
	if n := f.effects(t); n != 3 {
		t.Fatalf("side effects = %d, want 3", n)
	}
	if _, _, err := f.st.Do(ctx, Scope{f.mid, "payment.create", "bad key"}, h, effect); !errors.Is(err, ErrBadKey) {
		t.Fatalf("want ErrBadKey, got %v", err)
	}
}

func TestDoFailureRollsBackAndFreesKey(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := Scope{f.mid, "payment.create", "key-0000002"}
	h := hash(t, `{"a":1}`)
	boom := errors.New("boom")
	_, _, err := f.st.Do(ctx, sc, h, func(ctx context.Context, tx pgx.Tx) (Response, error) {
		if _, err := effect(ctx, tx); err != nil {
			return Response{}, err
		}
		return Response{}, boom
	})
	if !errors.Is(err, boom) || f.effects(t) != 0 {
		t.Fatalf("err=%v effects=%d", err, f.effects(t))
	}
	if _, replayed, err := f.st.Do(ctx, sc, h, effect); err != nil || replayed || f.effects(t) != 1 {
		t.Fatalf("retry: replayed=%v err=%v effects=%d", replayed, err, f.effects(t))
	}
}

// 40 parallel requests with the SAME key: the side effect happens exactly once.
func TestDoConcurrentSameKey(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := Scope{f.mid, "payment.create", "key-concurrent-1"}
	h := hash(t, `{"amount":10000}`)

	const n = 40
	var wg sync.WaitGroup
	var fresh, replays atomic.Int32
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, replayed, err := f.st.Do(ctx, sc, h, effect)
			if err != nil {
				t.Errorf("unexpected: %v", err)
				return
			}
			if replayed {
				replays.Add(1)
			} else {
				fresh.Add(1)
			}
			ids <- r.ResourceID
		}()
	}
	wg.Wait()
	close(ids)
	if fresh.Load() != 1 || replays.Load() != n-1 {
		t.Fatalf("fresh=%d replays=%d", fresh.Load(), replays.Load())
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Fatalf("different resource ids: %s vs %s", first, id)
		}
	}
	if f.effects(t) != 1 {
		t.Fatalf("side effects = %d, want 1", f.effects(t))
	}
}

// Parallel requests with DIFFERENT keys all execute; same key + different payload conflicts under load.
func TestDoConcurrentDistinctKeysAndConflicts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := Scope{f.mid, "payment.create", "distinct-key-" + strconv.Itoa(i)}
			if _, replayed, err := f.st.Do(ctx, sc, hash(t, `{"i":`+strconv.Itoa(i)+`}`), effect); err != nil || replayed {
				t.Errorf("i=%d replayed=%v err=%v", i, replayed, err)
			}
		}()
	}
	wg.Wait()
	if f.effects(t) != 20 {
		t.Fatalf("effects = %d, want 20", f.effects(t))
	}

	sc := Scope{f.mid, "payment.create", "key-conflict-1"}
	var ok, conflict atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.st.Do(ctx, sc, hash(t, `{"v":`+strconv.Itoa(i%2)+`}`), effect)
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrConflict):
				conflict.Add(1)
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	// Exactly one payload variant wins the key; the other variant always conflicts.
	if ok.Load() != 10 || conflict.Load() != 10 || f.effects(t) != 21 {
		t.Fatalf("ok=%d conflict=%d effects=%d", ok.Load(), conflict.Load(), f.effects(t))
	}
}

// Reserve/Complete (external-call flow): one winner, others see in_progress, then the stored result.
func TestReserveCompleteConcurrent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := Scope{f.mid, "payment.create", "key-reserve-01"}
	h := hash(t, `{"amount":5}`)

	const n = 30
	var wg sync.WaitGroup
	var winners, inProgress atomic.Int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prev, err := f.st.Reserve(ctx, sc, h)
			switch {
			case err == nil && prev == nil:
				winners.Add(1)
			case errors.Is(err, ErrInProgress):
				inProgress.Add(1)
			default:
				t.Errorf("unexpected prev=%v err=%v", prev, err)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 || inProgress.Load() != n-1 {
		t.Fatalf("winners=%d inProgress=%d", winners.Load(), inProgress.Load())
	}
	// The crashed/slow holder stays in_progress: a retry must not re-run it.
	if _, err := f.st.Reserve(ctx, sc, h); !errors.Is(err, ErrInProgress) {
		t.Fatalf("want ErrInProgress, got %v", err)
	}
	if _, err := f.st.Reserve(ctx, sc, hash(t, `{"amount":6}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	if err := f.st.Complete(ctx, sc, Response{Status: 201, Body: json.RawMessage(`{"ok":true}`), ResourceType: "payment", ResourceID: "p1"}); err != nil {
		t.Fatal(err)
	}
	prev, err := f.st.Reserve(ctx, sc, h)
	if err != nil || prev == nil || prev.ResourceID != "p1" || prev.Status != 201 {
		t.Fatalf("replay after complete: %+v err=%v", prev, err)
	}
}

func TestExpiredCompletedKeyCanBeReused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sc := Scope{f.mid, "payment.create", "key-expire-01"}
	h := hash(t, `{"a":1}`)
	if _, _, err := f.st.Do(ctx, sc, h, effect); err != nil {
		t.Fatal(err)
	}
	f.st.now = func() time.Time { return time.Now().Add(DefaultTTL + time.Hour) }
	if _, replayed, err := f.st.Do(ctx, sc, h, effect); err != nil || replayed {
		t.Fatalf("after expiry: replayed=%v err=%v", replayed, err)
	}
	if f.effects(t) != 2 {
		t.Fatalf("effects = %d, want 2", f.effects(t))
	}
}
