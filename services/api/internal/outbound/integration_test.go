package outbound

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/secretbox"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/testdb"
)

type env struct {
	db  *pgxpool.Pool
	svc *Service
	a   string // merchant A
	b   string // merchant B
}

func setup(t *testing.T) env {
	t.Helper()
	db := testdb.New(t)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	box, _ := secretbox.New(key)
	svc := NewService(db, box, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.allowPrivate = true // httptest listens on loopback
	svc.client = safeClient(2*time.Second, true)
	a, _ := testdb.SeedMerchant(t, db, "A")
	b, _ := testdb.SeedMerchant(t, db, "B")
	return env{db, svc, a, b}
}

// enqueue creates an event for merchant m the way payment.Transition does.
func (e env) enqueue(t *testing.T, m, typ, resource string) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := EnqueueTx(ctx, tx, m, typ, resource, map[string]any{"payment": map[string]any{"id": resource}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func (e env) one(t *testing.T, q string, args ...any) (n int) {
	t.Helper()
	if err := e.db.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) due(t *testing.T) { // make every pending delivery due now
	t.Helper()
	if _, err := e.db.Exec(context.Background(), `UPDATE webhook_deliveries SET next_attempt_at = now() WHERE status = 'pending'`); err != nil {
		t.Fatal(err)
	}
}

func newID(i int) string {
	b := []byte("11111111-1111-4111-8111-000000000000")
	for p, v := len(b)-1, i; v > 0; p, v = p-1, v/10 {
		b[p] = byte('0' + v%10)
	}
	return string(b)
}

func TestDeliverySignedAndSucceeds(t *testing.T) {
	e := setup(t)
	var got struct {
		body []byte
		sig  string
		evID string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.body, _ = io.ReadAll(r.Body)
		got.sig, got.evID = r.Header.Get(SignatureHeader), r.Header.Get("X-EraPay-Event-Id")
		w.WriteHeader(204)
	}))
	defer srv.Close()
	ep, err := e.svc.CreateEndpoint(context.Background(), e.a, srv.URL, Actor{})
	if err != nil || ep.Secret == "" {
		t.Fatalf("create: %v", err)
	}
	e.enqueue(t, e.a, "payment.succeeded", newID(1))
	if n, err := e.svc.DispatchOnce(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("dispatch n=%d err=%v", n, err)
	}
	if err := Verify(ep.Secret, got.sig, got.body, time.Now(), time.Minute); err != nil {
		t.Fatalf("receiver cannot verify signature: %v", err)
	}
	var env struct {
		ID, Type string
		Data     json.RawMessage
	}
	if err := json.Unmarshal(got.body, &env); err != nil || env.Type != "payment.succeeded" || env.ID != got.evID || env.ID == "" {
		t.Fatalf("envelope %s (%v)", got.body, err)
	}
	if e.one(t, `SELECT count(*) FROM webhook_deliveries WHERE status='succeeded' AND attempts=1 AND last_http_status=204`) != 1 ||
		e.one(t, `SELECT count(*) FROM webhook_delivery_attempts`) != 1 {
		t.Fatal("delivery/attempt not recorded")
	}
	// Nothing left to send.
	if n, _ := e.svc.DispatchOnce(context.Background(), 10); n != 0 {
		t.Fatalf("redispatched %d", n)
	}
}

func TestSecretIsEncryptedAtRest(t *testing.T) {
	e := setup(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	ep, err := e.svc.CreateEndpoint(context.Background(), e.a, srv.URL, Actor{})
	if err != nil {
		t.Fatal(err)
	}
	if e.one(t, `SELECT count(*) FROM webhook_endpoints WHERE position($1::bytea in secret_enc) > 0`, []byte(ep.Secret)) != 0 {
		t.Fatal("plaintext secret stored")
	}
	list, _ := e.svc.ListEndpoints(context.Background(), e.a)
	for _, x := range list {
		if x.Secret != "" {
			t.Fatal("list leaks secret")
		}
	}
}

func TestRetriesBackOffThenDeadLetter(t *testing.T) {
	e := setup(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(500)
	}))
	defer srv.Close()
	if _, err := e.svc.CreateEndpoint(context.Background(), e.a, srv.URL, Actor{}); err != nil {
		t.Fatal(err)
	}
	e.enqueue(t, e.a, "payment.failed", newID(2))

	e.svc.DispatchOnce(context.Background(), 10) //nolint:errcheck
	var attempts int
	var next time.Time
	if err := e.db.QueryRow(context.Background(), `SELECT attempts, next_attempt_at FROM webhook_deliveries`).Scan(&attempts, &next); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || !next.After(time.Now().Add(20*time.Second)) {
		t.Fatalf("first failure: attempts=%d next in %v (want backoff >= ~24s)", attempts, time.Until(next))
	}
	// Not due yet: a dispatch before the backoff elapses must not send.
	if n, _ := e.svc.DispatchOnce(context.Background(), 10); n != 0 || calls.Load() != 1 {
		t.Fatalf("sent before backoff elapsed: n=%d calls=%d", n, calls.Load())
	}
	for i := 1; i < MaxAttempts; i++ {
		e.due(t)
		e.svc.DispatchOnce(context.Background(), 10) //nolint:errcheck
	}
	if calls.Load() != MaxAttempts {
		t.Fatalf("calls = %d, want %d", calls.Load(), MaxAttempts)
	}
	var status, lastErr string
	var last int
	e.db.QueryRow(context.Background(), `SELECT status, last_error, last_http_status FROM webhook_deliveries`).Scan(&status, &lastErr, &last) //nolint:errcheck
	if status != "dead" || lastErr != "http_500" || last != 500 {
		t.Fatalf("status=%s err=%s http=%d", status, lastErr, last)
	}
	if e.one(t, `SELECT count(*) FROM webhook_delivery_attempts`) != MaxAttempts {
		t.Fatal("attempt history incomplete")
	}
	e.due(t)
	if n, _ := e.svc.DispatchOnce(context.Background(), 10); n != 0 {
		t.Fatal("dead delivery was retried")
	}
}

func TestRedirectIsNotFollowedAndCountsAsFailure(t *testing.T) {
	e := setup(t)
	var internal atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { internal.Add(1) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	e.svc.CreateEndpoint(context.Background(), e.a, srv.URL, Actor{}) //nolint:errcheck
	e.enqueue(t, e.a, "payment.succeeded", newID(3))
	e.svc.DispatchOnce(context.Background(), 10) //nolint:errcheck
	var s, le string
	e.db.QueryRow(context.Background(), `SELECT status, last_error FROM webhook_deliveries`).Scan(&s, &le) //nolint:errcheck
	if internal.Load() != 0 || s != "pending" || le != "redirect_not_followed" {
		t.Fatalf("redirect: hits=%d status=%s err=%s", internal.Load(), s, le)
	}
}

// Many dispatchers racing over the same backlog: every delivery is sent exactly once (SKIP LOCKED + lease).
func TestConcurrentDispatchersSendEachDeliveryOnce(t *testing.T) {
	e := setup(t)
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Header.Get("X-EraPay-Event-Id")]++
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}))
	defer srv.Close()
	e.svc.CreateEndpoint(context.Background(), e.a, srv.URL, Actor{}) //nolint:errcheck
	const events = 40
	for i := 1; i <= events; i++ {
		e.enqueue(t, e.a, "payment.succeeded", newID(100+i))
	}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				n, err := e.svc.DispatchOnce(context.Background(), 5)
				if err != nil {
					t.Errorf("dispatch: %v", err)
					return
				}
				if n == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()
	if len(seen) != events {
		t.Fatalf("delivered %d distinct events, want %d", len(seen), events)
	}
	for id, c := range seen {
		if c != 1 {
			t.Errorf("event %s sent %d times", id, c)
		}
	}
	if e.one(t, `SELECT count(*) FROM webhook_deliveries WHERE status='succeeded'`) != events {
		t.Fatal("not all marked succeeded")
	}
}

func TestEnqueueIsIdempotentPerEventAndFansOutToActiveEndpointsOnly(t *testing.T) {
	e := setup(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	ctx := context.Background()
	e1, _ := e.svc.CreateEndpoint(ctx, e.a, srv.URL+"/1", Actor{})
	e.svc.CreateEndpoint(ctx, e.a, srv.URL+"/2", Actor{})            //nolint:errcheck
	e.svc.CreateEndpoint(ctx, e.b, srv.URL+"/other-tenant", Actor{}) //nolint:errcheck
	if err := e.svc.Disable(ctx, e.a, e1.ID, Actor{}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); e.enqueue(t, e.a, "payment.succeeded", newID(7)) }()
	}
	wg.Wait()
	if e.one(t, `SELECT count(*) FROM webhook_events`) != 1 || e.one(t, `SELECT count(*) FROM webhook_deliveries`) != 1 {
		t.Fatal("duplicate enqueue produced extra events/deliveries, or fanned out to a disabled/foreign endpoint")
	}
	// A different final status of another payment is a different event.
	e.enqueue(t, e.a, "payment.expired", newID(8))
	if e.one(t, `SELECT count(*) FROM webhook_events`) != 2 {
		t.Fatal("distinct event deduplicated")
	}
}

func TestRedeliverKeepsHistoryAndIsTenantScoped(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(503)
		}
	}))
	defer srv.Close()
	ep, _ := e.svc.CreateEndpoint(ctx, e.a, srv.URL, Actor{})
	e.svc.CreateEndpoint(ctx, e.b, srv.URL, Actor{}) //nolint:errcheck
	e.enqueue(t, e.a, "payment.succeeded", newID(9))
	for i := 0; i < MaxAttempts; i++ {
		e.due(t)
		e.svc.DispatchOnce(ctx, 10) //nolint:errcheck
	}
	ds, _ := e.svc.ListDeliveries(ctx, e.a, DeliveryFilter{Status: "dead"})
	if len(ds) != 1 {
		t.Fatalf("dead deliveries = %d", len(ds))
	}
	orig := ds[0].ID

	// Tenant B can neither see nor redeliver, rotate or disable A's resources.
	if bd, _ := e.svc.ListDeliveries(ctx, e.b, DeliveryFilter{}); len(bd) != 0 {
		t.Fatal("tenant B sees tenant A deliveries")
	}
	if _, err := e.svc.Redeliver(ctx, e.b, orig, Actor{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign redeliver: %v", err)
	}
	if _, err := e.svc.RotateSecret(ctx, e.b, ep.ID, Actor{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign rotate: %v", err)
	}
	if err := e.svc.Disable(ctx, e.b, ep.ID, Actor{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign disable: %v", err)
	}

	fail.Store(false)
	nd, err := e.svc.Redeliver(ctx, e.a, orig, Actor{})
	if err != nil || nd.Status != "pending" || nd.RedeliveryOf == nil || *nd.RedeliveryOf != orig {
		t.Fatalf("redeliver: %+v %v", nd, err)
	}
	if _, err := e.svc.Redeliver(ctx, e.a, nd.ID, Actor{}); !errors.Is(err, ErrNotRedeliver) {
		t.Fatalf("pending delivery redelivered: %v", err)
	}
	if n, _ := e.svc.DispatchOnce(ctx, 10); n != 1 {
		t.Fatalf("redelivery dispatched %d", n)
	}
	if e.one(t, `SELECT count(*) FROM webhook_deliveries WHERE status='succeeded'`) != 1 ||
		e.one(t, `SELECT count(*) FROM webhook_deliveries WHERE status='dead'`) != 1 {
		t.Fatal("original history not preserved next to the successful redelivery")
	}
}

func TestDisableCancelsPendingAndRotateSwitchesSecret(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var sig, body atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		sig.Store(r.Header.Get(SignatureHeader))
		body.Store(b)
	}))
	defer srv.Close()
	ep, _ := e.svc.CreateEndpoint(ctx, e.a, srv.URL, Actor{})
	rot, err := e.svc.RotateSecret(ctx, e.a, ep.ID, Actor{})
	if err != nil || rot.Secret == ep.Secret || rot.SecretHint == "" {
		t.Fatalf("rotate: %v", err)
	}
	e.enqueue(t, e.a, "payment.succeeded", newID(11))
	e.svc.DispatchOnce(ctx, 10) //nolint:errcheck
	if Verify(rot.Secret, sig.Load().(string), body.Load().([]byte), time.Now(), time.Minute) != nil ||
		Verify(ep.Secret, sig.Load().(string), body.Load().([]byte), time.Now(), time.Minute) == nil {
		t.Fatal("delivery not signed with the rotated secret only")
	}

	e.enqueue(t, e.a, "payment.expired", newID(12))
	if err := e.svc.Disable(ctx, e.a, ep.ID, Actor{}); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.DispatchOnce(ctx, 10); n != 0 {
		t.Fatal("disabled endpoint still received a delivery")
	}
	if e.one(t, `SELECT count(*) FROM webhook_deliveries WHERE status='cancelled'`) != 1 {
		t.Fatal("pending delivery not cancelled")
	}
}

func TestEndpointLimitHoldsUnderConcurrency(t *testing.T) {
	e := setup(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	var wg sync.WaitGroup
	var ok, limited atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.CreateEndpoint(context.Background(), e.a, srv.URL, Actor{})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrTooManyEndpts):
				limited.Add(1)
			default:
				t.Errorf("create: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != MaxActiveEndpoints || limited.Load() != 12-MaxActiveEndpoints {
		t.Fatalf("ok=%d limited=%d", ok.Load(), limited.Load())
	}
}

func TestSSRFBlockedAtRegistrationAndAtSend(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	prod := NewService(e.db, e.svc.box, e.svc.log) // production settings: allowPrivate=false
	prod.resolver = fakeResolver{"ok.example": {"93.184.216.34"}}
	for _, u := range []string{"http://ok.example/x", "https://127.0.0.1/x", "https://169.254.169.254/x"} {
		if _, err := prod.CreateEndpoint(ctx, e.a, u, Actor{}); err == nil {
			t.Errorf("registered %s", u)
		}
	}
	// An endpoint that was valid at registration but now resolves to a private address is not sent to.
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
	defer srv.Close()
	ep, err := prod.CreateEndpoint(ctx, e.a, "https://ok.example/hook", Actor{})
	if err != nil {
		t.Fatal(err)
	}
	e.enqueue(t, e.a, "payment.succeeded", newID(13))
	prod.resolver = fakeResolver{"ok.example": {"10.0.0.7"}} // DNS changed after registration
	prod.DispatchOnce(ctx, 10)                               //nolint:errcheck
	var le string
	e.db.QueryRow(ctx, `SELECT last_error FROM webhook_deliveries WHERE endpoint_id=$1`, ep.ID).Scan(&le) //nolint:errcheck
	if le != "blocked_destination" || hits.Load() != 0 {
		t.Fatalf("last_error=%q hits=%d", le, hits.Load())
	}
}

// A rolled-back transition must not leak a webhook event (transactional outbox).
func TestRollbackLeavesNoEvent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	tx, _ := e.db.Begin(ctx)
	if err := EnqueueTx(ctx, tx, e.a, "payment.succeeded", newID(14), map[string]any{}); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx) //nolint:errcheck
	if e.one(t, `SELECT count(*) FROM webhook_events`) != 0 {
		t.Fatal("event survived rollback")
	}
}
