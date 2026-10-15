package payment

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/idempotency"
)

// fakeGW is a TEST-ONLY adapter used to drive the orchestrator through every outcome.
// It is not registered anywhere in production code.
type fakeGW struct {
	createCalls, queryCalls atomic.Int32
	create                  func(gateway.CreateRequest) (gateway.CreateResult, error)
	query                   func(gateway.QueryRequest) (gateway.QueryResult, error)
	mapping                 map[string]gateway.Status
}

func (f *fakeGW) Code() string { return "gw1" }
func (f *fakeGW) Capabilities() gateway.Capabilities {
	return gateway.Capabilities{PaymentMethods: []string{"QRIS"}, Inquiry: true, Cancel: true}
}
func (f *fakeGW) CreatePayment(_ context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	f.createCalls.Add(1)
	return f.create(r)
}
func (f *fakeGW) GetPayment(_ context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	f.queryCalls.Add(1)
	return f.query(r)
}
func (f *fakeGW) CancelPayment(context.Context, gateway.CancelRequest) error { return nil }
func (f *fakeGW) MapStatus(raw string) (gateway.Status, bool) {
	s, ok := f.mapping[raw]
	return s, ok
}

func okCreate(r gateway.CreateRequest) (gateway.CreateResult, error) {
	return gateway.CreateResult{GatewayReference: "GW-" + r.Reference, QRPayload: "QR-" + r.Reference, ResponseCode: "2004700"}, nil
}

func newFake() *fakeGW {
	return &fakeGW{create: okCreate, mapping: map[string]gateway.Status{
		"PAID": gateway.StatusSucceeded, "WAIT": gateway.StatusPending, "DEAD": gateway.StatusFailed}}
}

type oenv struct {
	env
	orch *Orchestrator
	gw   *fakeGW
}

func osetup(t *testing.T) oenv {
	t.Helper()
	e := setup(t)
	if _, err := e.db.Exec(context.Background(), `INSERT INTO merchant_gateway_accounts
		(merchant_id, gateway, environment, gateway_ref, activation_status) VALUES ($1,'gw1','sandbox','MID-1','active')`, e.mid); err != nil {
		t.Fatal(err)
	}
	gw := newFake()
	reg := gateway.NewRegistry()
	reg.Register(gw)
	o := NewOrchestrator(e.svc, e.db, idempotency.NewStore(e.db), reg, "sandbox", slog.New(slog.NewTextHandler(io.Discard, nil)))
	return oenv{e, o, gw}
}

func (e oenv) req(key string, amount int64) CreateRequest {
	h, _ := idempotency.HashJSON([]byte(`{"amount":`+itoa(amount)+`}`), "POST", "/v1/payments")
	return CreateRequest{MerchantID: e.mid, OutletID: e.oid, MerchantReference: "ORD-1", Amount: amount, Currency: "IDR",
		PaymentMethod: "QRIS", IdempotencyKey: key, RequestHash: h}
}

func itoa(n int64) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func TestCreateHappyPathAndIdempotentReplay(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	out, err := e.orch.Create(ctx, e.req("key-happy-01", 10000))
	if err != nil || out.Replayed {
		t.Fatalf("%+v %v", out, err)
	}
	p := out.Payment
	if p.Status != Pending || p.QRCode == nil || p.QRCode.Value != "QR-"+p.ID || p.QRCode.Type != "provider_defined" || p.Gateway != "gw1" {
		t.Fatalf("%+v", p)
	}
	if p.Fee.MDRFee != 70 || p.Fee.PlatformFee != 100 || p.Fee.MerchantNet != 9830 {
		t.Fatalf("fee: %+v", p.Fee)
	}
	if n := e.count(t, `SELECT count(*) FROM payment_attempts WHERE payment_id=$1 AND operation='create' AND outcome='ok'`, p.ID); n != 1 {
		t.Fatalf("attempts = %d", n)
	}
	var next *time.Time
	_ = e.db.QueryRow(ctx, `SELECT next_inquiry_at FROM payments WHERE id=$1`, p.ID).Scan(&next)
	if next == nil || time.Until(*next) < 30*time.Second {
		t.Fatalf("first inquiry should be ~60s out: %v", next)
	}

	again, err := e.orch.Create(ctx, e.req("key-happy-01", 10000))
	if err != nil || !again.Replayed || again.Payment.ID != p.ID {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if e.gw.createCalls.Load() != 1 {
		t.Fatalf("gateway called %d times", e.gw.createCalls.Load())
	}
	if _, err := e.orch.Create(ctx, e.req("key-happy-01", 20000)); !errors.Is(err, idempotency.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	if e.gw.createCalls.Load() != 1 || e.count(t, `SELECT count(*) FROM payments`) != 1 {
		t.Fatal("conflicting retry had side effects")
	}
}

// 20 parallel POSTs with one key: ONE gateway call, ONE payment; the rest get the same payment or in-progress.
func TestCreateConcurrentSameKey(t *testing.T) {
	e := osetup(t)
	e.gw.create = func(r gateway.CreateRequest) (gateway.CreateResult, error) {
		time.Sleep(80 * time.Millisecond) // widen the race window
		return okCreate(r)
	}
	var wg sync.WaitGroup
	var inProgress atomic.Int32
	ids := make(chan string, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := e.orch.Create(context.Background(), e.req("key-race-0001", 5000))
			switch {
			case err == nil:
				ids <- out.Payment.ID
			case errors.Is(err, idempotency.ErrInProgress):
				inProgress.Add(1)
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 || e.gw.createCalls.Load() != 1 || e.count(t, `SELECT count(*) FROM payments`) != 1 {
		t.Fatalf("distinct=%d gatewayCalls=%d payments=%d inProgress=%d", len(seen), e.gw.createCalls.Load(),
			e.count(t, `SELECT count(*) FROM payments`), inProgress.Load())
	}
}

func TestGatewayRejectMarksFailed(t *testing.T) {
	e := osetup(t)
	e.gw.create = func(gateway.CreateRequest) (gateway.CreateResult, error) {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Code: "4004700", HTTPStatus: 400, Message: "bad"}
	}
	out, err := e.orch.Create(context.Background(), e.req("key-reject-01", 5000))
	if err != nil || out.Payment.Status != Failed || out.Payment.QRCode != nil {
		t.Fatalf("%+v %v", out.Payment, err)
	}
	if e.payable(t) != 0 {
		t.Fatal("money moved on a rejected payment")
	}
}

// Invariant #3: ambiguity is `unknown`, create is never retried, and a retry of the
// same request returns the same payment without calling the gateway again.
func TestAmbiguousCreateIsUnknownAndNeverRetriedBlindly(t *testing.T) {
	e := osetup(t)
	e.gw.create = func(gateway.CreateRequest) (gateway.CreateResult, error) {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: 504, Retryable: true}
	}
	ctx := context.Background()
	out, err := e.orch.Create(ctx, e.req("key-timeout-01", 5000))
	if err != nil || out.Payment.Status != Unknown || out.Payment.FinalAt != nil {
		t.Fatalf("%+v %v", out.Payment, err)
	}
	again, err := e.orch.Create(ctx, e.req("key-timeout-01", 5000))
	if err != nil || again.Payment.ID != out.Payment.ID || again.Payment.Status != Unknown {
		t.Fatalf("%+v %v", again, err)
	}
	if e.gw.createCalls.Load() != 1 {
		t.Fatalf("create retried blindly: %d calls", e.gw.createCalls.Load())
	}
	if n := e.count(t, `SELECT count(*) FROM payment_attempts WHERE outcome='ambiguous'`); n != 1 {
		t.Fatalf("ambiguous attempts = %d", n)
	}
}

// The client hanging up mid-create must not strand the payment in `created`.
func TestClientDisconnectDoesNotLoseGatewayResult(t *testing.T) {
	e := osetup(t)
	e.gw.create = func(r gateway.CreateRequest) (gateway.CreateResult, error) {
		time.Sleep(150 * time.Millisecond)
		return okCreate(r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	out, err := e.orch.Create(ctx, e.req("key-cancel-001", 5000))
	if err != nil || out.Payment.Status != Pending {
		t.Fatalf("%+v %v", out.Payment, err)
	}
}

func TestRoutingUnavailable(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	for name, q := range map[string]string{
		"pending activation": `UPDATE merchant_gateway_accounts SET activation_status='pending_confirmation'`,
		"suspended":          `UPDATE merchant_gateway_accounts SET activation_status='suspended'`,
		"wrong environment":  `UPDATE merchant_gateway_accounts SET activation_status='active', environment='production'`,
		"no merchant id":     `UPDATE merchant_gateway_accounts SET environment='sandbox', gateway_ref=NULL`,
	} {
		if _, err := e.db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
		if _, err := e.orch.Create(ctx, e.req("key-route-0001", 5000)); !errors.Is(err, ErrMethodUnavailable) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if e.gw.createCalls.Load() != 0 || e.count(t, `SELECT count(*) FROM payments`) != 0 || e.count(t, `SELECT count(*) FROM idempotency_records`) != 0 {
		t.Fatal("unavailable method left side effects")
	}
	// An adapter that is not registered is unavailable too.
	if _, err := e.db.Exec(ctx, `UPDATE merchant_gateway_accounts SET environment='sandbox', gateway_ref='M', gateway='other', activation_status='active'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.orch.Create(ctx, e.req("key-route-0002", 5000)); !errors.Is(err, ErrMethodUnavailable) {
		t.Fatalf("unregistered gateway: %v", err)
	}
}

func TestPreGatewayFailureIsStoredAndNeverReachesGateway(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	r := e.req("key-invalid-001", 0) // amount 0 -> validation, deterministic
	if _, err := e.orch.Create(ctx, r); !errors.Is(err, ErrValidation) {
		t.Fatalf("%v", err)
	}
	if _, err := e.orch.Create(ctx, r); !errors.Is(err, ErrValidation) {
		t.Fatalf("replayed failure: %v", err)
	}
	if e.gw.createCalls.Load() != 0 || e.count(t, `SELECT count(*) FROM payments`) != 0 {
		t.Fatal("invalid request had side effects")
	}
	if _, err := e.orch.Create(ctx, CreateRequest{MerchantID: e.mid, PaymentMethod: "QRIS", Amount: 1000, Currency: "IDR",
		MerchantReference: "x", IdempotencyKey: "short"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad idempotency key: %v", err)
	}
}

func (e oenv) pending(t *testing.T, key string, amount int64) Payment {
	t.Helper()
	out, err := e.orch.Create(context.Background(), e.req(key, amount))
	if err != nil || out.Payment.Status != Pending {
		t.Fatalf("%+v %v", out.Payment, err)
	}
	return out.Payment
}

func TestInquiryAppliesConfirmedStatus(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	p := e.pending(t, "key-inq-000001", 10000)
	e.gw.query = func(r gateway.QueryRequest) (gateway.QueryResult, error) {
		if r.GatewayReference != "GW-"+p.ID || r.AccountRef != "MID-1" || r.Reference != p.ID {
			t.Errorf("query request: %+v", r)
		}
		return gateway.QueryResult{RawStatus: "PAID", Amount: 10000, Currency: "IDR"}, nil
	}
	if err := e.orch.Inquire(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, e.mid, p.ID)
	if got.Status != Succeeded || e.payable(t) != 9830 {
		t.Fatalf("status=%s payable=%d", got.Status, e.payable(t))
	}
	if n := e.count(t, `SELECT count(*) FROM payment_events WHERE payment_id=$1 AND source='inquiry' AND applied`, p.ID); n != 1 {
		t.Fatalf("inquiry events = %d", n)
	}
	if err := e.orch.Inquire(ctx, p.ID); err != nil || e.gw.queryCalls.Load() != 1 { // final: not asked again
		t.Fatalf("final payment re-inquired: %v calls=%d", err, e.gw.queryCalls.Load())
	}
}

func TestInquiryWithoutGatewayReference(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	p := e.pending(t, "key-inq-noref-01", 10000)
	if _, err := e.db.Exec(ctx, `UPDATE payments SET status='created', gateway_reference=NULL WHERE id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.orch.Inquire(ctx, p.ID); !errors.Is(err, ErrNotSentToGateway) {
		t.Fatalf("want ErrNotSentToGateway, got %v", err)
	}
	if e.gw.queryCalls.Load() != 0 {
		t.Fatal("gateway must not be queried without a reference")
	}
}

func TestInquirySafetyRails(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()

	// amount mismatch on "success": NOT applied, no money moves, evidence kept.
	p := e.pending(t, "key-inq-mismatch", 10000)
	e.gw.query = func(gateway.QueryRequest) (gateway.QueryResult, error) {
		return gateway.QueryResult{RawStatus: "PAID", Amount: 9000, Currency: "IDR"}, nil
	}
	if err := e.orch.Inquire(ctx, p.ID); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("%v", err)
	}
	if got, _ := e.svc.Get(ctx, e.mid, p.ID); got.Status != Pending || e.payable(t) != 0 {
		t.Fatalf("mismatch applied: %s payable=%d", got.Status, e.payable(t))
	}
	if e.count(t, `SELECT count(*) FROM payment_attempts WHERE payment_id=$1 AND outcome='amount_mismatch'`, p.ID) != 1 {
		t.Fatal("mismatch not recorded")
	}
	// unusable amount (-1) is a mismatch as well
	e.gw.query = func(gateway.QueryRequest) (gateway.QueryResult, error) {
		return gateway.QueryResult{RawStatus: "PAID", Amount: -1}, nil
	}
	if err := e.orch.Inquire(ctx, p.ID); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("%v", err)
	}

	// unmapped raw status: unchanged.
	e.gw.query = func(gateway.QueryRequest) (gateway.QueryResult, error) {
		return gateway.QueryResult{RawStatus: "??"}, nil
	}
	if err := e.orch.Inquire(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(ctx, e.mid, p.ID); got.Status != Pending {
		t.Fatalf("unmapped status changed payment: %s", got.Status)
	}
	if e.count(t, `SELECT count(*) FROM payment_attempts WHERE payment_id=$1 AND outcome='unmapped'`, p.ID) != 1 {
		t.Fatal("unmapped not recorded")
	}

	// ambiguous inquiry (timeout): unchanged, no failure.
	e.gw.query = func(gateway.QueryRequest) (gateway.QueryResult, error) {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: 504}
	}
	if err := e.orch.Inquire(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(ctx, e.mid, p.ID); got.Status != Pending {
		t.Fatalf("timeout changed payment: %s", got.Status)
	}
}

func TestUnknownResolvedByInquiry(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	p := e.pending(t, "key-unknown-001", 10000)
	e.move(t, p.ID, Unknown, SourceInternal)
	e.gw.query = func(gateway.QueryRequest) (gateway.QueryResult, error) {
		return gateway.QueryResult{RawStatus: "DEAD", Amount: 10000, Currency: "IDR"}, nil
	}
	if err := e.orch.Inquire(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(ctx, e.mid, p.ID); got.Status != Failed {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestPollOnceSchedulingBackoffAndConcurrency(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	e.gw.query = func(gateway.QueryRequest) (gateway.QueryResult, error) {
		time.Sleep(20 * time.Millisecond)
		return gateway.QueryResult{RawStatus: "WAIT", Amount: 1000, Currency: "IDR"}, nil
	}
	var ids []string
	for i := 0; i < 6; i++ {
		ids = append(ids, e.pending(t, "key-poll-"+itoa(int64(i))+"0000", 1000).ID)
	}
	if n, _ := e.orch.PollOnce(ctx, 10); n != 0 {
		t.Fatalf("polled %d payments before the 60s mark", n)
	}
	if _, err := e.db.Exec(ctx, `UPDATE payments SET next_inquiry_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	// 5 pollers at once: each due payment is claimed by exactly one of them.
	var wg sync.WaitGroup
	var total atomic.Int32
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := e.orch.PollOnce(ctx, 3)
			if err != nil {
				t.Errorf("poll: %v", err)
			}
			total.Add(int32(n))
		}()
	}
	wg.Wait()
	if total.Load() != 6 || e.gw.queryCalls.Load() != 6 {
		t.Fatalf("claimed=%d queries=%d, want 6/6", total.Load(), e.gw.queryCalls.Load())
	}
	for _, id := range ids {
		var attempts int
		var next time.Time
		_ = e.db.QueryRow(ctx, `SELECT inquiry_attempts, next_inquiry_at FROM payments WHERE id=$1`, id).Scan(&attempts, &next)
		if attempts != 1 || time.Until(next) < 20*time.Second {
			t.Fatalf("backoff not scheduled: attempts=%d next in %v", attempts, time.Until(next))
		}
	}
	if n, _ := e.orch.PollOnce(ctx, 10); n != 0 {
		t.Fatalf("re-polled %d before backoff elapsed", n)
	}
}

func TestSweepStaleCreated(t *testing.T) {
	e := osetup(t)
	ctx := context.Background()
	fresh := e.create(t, "fresh", 1000)
	stale := e.create(t, "stale", 1000)
	// created_at is not covered by the immutability trigger.
	if _, err := e.db.Exec(ctx, `UPDATE payments SET created_at = now() - interval '10 minutes' WHERE id = $1`, stale.ID); err != nil {
		t.Fatal(err)
	}
	n, err := e.orch.SweepStaleCreated(ctx, 10)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if g, _ := e.svc.Get(ctx, e.mid, stale.ID); g.Status != Unknown {
		t.Fatalf("stale: %s", g.Status)
	}
	if g, _ := e.svc.Get(ctx, e.mid, fresh.ID); g.Status != Created {
		t.Fatalf("fresh: %s", g.Status)
	}
}
