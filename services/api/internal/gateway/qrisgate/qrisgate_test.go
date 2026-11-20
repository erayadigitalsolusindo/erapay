package qrisgate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const (
	merchantA = "11111111-1111-1111-1111-111111111111"
	merchantB = "22222222-2222-2222-2222-222222222222"
	payID     = "33333333-3333-3333-3333-333333333333"
)

type creds map[string][2]string

func (c creds) Credentials(_ context.Context, id string) (string, string, error) {
	v, ok := c[id]
	if !ok {
		return "", "", errors.New("not configured")
	}
	return v[0], v[1], nil
}

var testCreds = creds{merchantA: {"qg_live_keyA", "secretA-0123456789"}, merchantB: {"qg_live_keyB", "secretB-0123456789"}}

func newAdapter(t *testing.T, h http.HandlerFunc) (*Adapter, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a, err := New(Config{BaseURL: srv.URL, Creds: testCreds, Now: func() time.Time { return time.Unix(1_700_000_000, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	return a, srv
}

func createReq() gateway.CreateRequest {
	return gateway.CreateRequest{Reference: payID, AccountRef: merchantA, Method: "QRIS", Amount: 25_000, Currency: "IDR"}
}

func TestCreateSuccessUsesMerchantKeyAndKeepsBaseAmount(t *testing.T) {
	a, _ := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer qg_live_keyA" || r.URL.Path != "/api/v1/transactions" {
			t.Errorf("auth %q path %q", r.Header.Get("Authorization"), r.URL.Path)
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"data":{"transaction_id":"tx-1","base_amount":25000,"unique_code":7,"total_amount":25007,"status":"PENDING","qr_string":"000201QR"}}`))
	})
	res, err := a.CreatePayment(context.Background(), createReq())
	if err != nil || res.GatewayReference != "tx-1" || res.QRPayload != "000201QR" || res.RawStatus != "PENDING" {
		t.Fatalf("%+v %v", res, err)
	}
	if st, ok := a.MapStatus(res.RawStatus); !ok || st != gateway.StatusPending {
		t.Fatalf("PENDING maps to %v %v", st, ok)
	}
}

func TestCreateFailureClassification(t *testing.T) {
	for code, want := range map[int]gateway.Kind{401: gateway.Rejected, 422: gateway.Rejected, 429: gateway.Ambiguous, 500: gateway.Ambiguous, 409: gateway.Ambiguous, 408: gateway.Ambiguous} {
		a, _ := newAdapter(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) })
		_, err := a.CreatePayment(context.Background(), createReq())
		var ge *gateway.Error
		if !errors.As(err, &ge) || ge.Kind != want {
			t.Errorf("http %d: %v; want kind %v", code, err, want)
		}
	}
	// A timeout is Ambiguous: the request may have landed (never "failed").
	a, srv := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close()
	if _, err := a.CreatePayment(context.Background(), createReq()); !gateway.IsAmbiguous(err) {
		t.Errorf("transport failure: %v; want ambiguous", err)
	}
	// A merchant without credentials is rejected before any call.
	req := createReq()
	req.AccountRef = "44444444-4444-4444-4444-444444444444"
	if _, err := a.CreatePayment(context.Background(), req); !gateway.IsRejected(err) {
		t.Errorf("no credentials: %v; want rejected", err)
	}
	// An incomplete or amount-changing answer is never trusted as a created QR.
	a, _ = newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"transaction_id":"x","base_amount":1,"qr_string":"q"}}`))
	})
	if _, err := a.CreatePayment(context.Background(), createReq()); !gateway.IsAmbiguous(err) {
		t.Errorf("amount changed: %v", err)
	}
}

func TestGetPaymentMapsStatusAndNeedsGatewayReference(t *testing.T) {
	a, _ := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/transactions/tx-1" {
			t.Errorf("path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":{"transaction_id":"tx-1","base_amount":25000,"status":"PAID","paid_at":"2026-11-20T10:00:00Z"}}`))
	})
	q, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: payID, GatewayReference: "tx-1", AccountRef: merchantA})
	if st, ok := a.MapStatus(q.RawStatus); err != nil || !ok || st != gateway.StatusSucceeded || q.Amount != 25_000 || q.PaidAt == nil {
		t.Fatalf("%+v %v", q, err)
	}
	if _, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: payID, AccountRef: merchantA}); !gateway.IsAmbiguous(err) {
		t.Fatalf("no gateway reference: %v; want ambiguous", err)
	}
	if _, ok := a.MapStatus("SOMETHING_NEW"); ok {
		t.Fatal("an unknown status must stay unmapped")
	}
}

func hook(secret string, ts int64, body, event, delivery string) http.Header {
	h := http.Header{}
	h.Set("X-QRISGate-Timestamp", strconv.FormatInt(ts, 10))
	h.Set("X-QRISGate-Signature", Signature(secret, ts, []byte(body)))
	h.Set("X-QRISGate-Event", event)
	h.Set("X-QRISGate-Delivery-Id", delivery)
	return h
}

func TestWebhookVerification(t *testing.T) {
	a, _ := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	body := `{"event":"payment.paid","transaction_id":"tx-1","external_order_id":"` + payID + `","base_amount":25000,"total_amount":25007}`
	pathA := "/webhooks/qrisgate/" + merchantA
	now := int64(1_700_000_000)

	ev, err := a.VerifyWebhook(hook("secretA-0123456789", now, body, "payment.paid", "d-1"), pathA, []byte(body))
	if err != nil || ev.PaymentRef != payID || ev.Status != gateway.StatusSucceeded || !ev.StatusOK || ev.Amount != 25_000 || ev.MerchantID != merchantA || ev.DedupKey != "qrisgate:webhook:d-1" {
		t.Fatalf("valid: %+v %v", ev, err)
	}
	// Retry with the same delivery id = same dedup key (duplicate webhook is a no-op downstream).
	ev2, _ := a.VerifyWebhook(hook("secretA-0123456789", now+30, body, "payment.paid", "d-1"), pathA, []byte(body))
	if ev2.DedupKey != ev.DedupKey {
		t.Fatalf("dedup keys differ: %q %q", ev.DedupKey, ev2.DedupKey)
	}
	// "sha256=" prefix is accepted.
	h := hook("secretA-0123456789", now, body, "payment.paid", "d-2")
	h.Set("X-QRISGate-Signature", "sha256="+h.Get("X-QRISGate-Signature"))
	if _, err := a.VerifyWebhook(h, pathA, []byte(body)); err != nil {
		t.Fatalf("prefixed signature: %v", err)
	}
	for name, c := range map[string]struct {
		h    http.Header
		path string
		body string
	}{
		"wrong secret":        {hook("nope-nope-nope-nope", now, body, "payment.paid", "d"), pathA, body},
		"other merchant path": {hook("secretA-0123456789", now, body, "payment.paid", "d"), "/webhooks/qrisgate/" + merchantB, body},
		"tampered body":       {hook("secretA-0123456789", now, body, "payment.paid", "d"), pathA, body + " "},
		"stale timestamp":     {hook("secretA-0123456789", now-301, body, "payment.paid", "d"), pathA, body},
		"future timestamp":    {hook("secretA-0123456789", now+301, body, "payment.paid", "d"), pathA, body},
		"no merchant in path": {hook("secretA-0123456789", now, body, "payment.paid", "d"), "/webhooks/qrisgate", body},
		"unknown merchant":    {hook("secretA-0123456789", now, body, "payment.paid", "d"), "/webhooks/qrisgate/44444444-4444-4444-4444-444444444444", body},
		"missing signature":   {http.Header{}, pathA, body},
	} {
		if _, err := a.VerifyWebhook(c.h, c.path, []byte(c.body)); !errors.Is(err, gateway.ErrInvalidSignature) {
			t.Errorf("%s: %v; want ErrInvalidSignature", name, err)
		}
	}
	// Expired and cancelled map; an unknown event stays unmapped (recorded, never applied).
	for ev, want := range map[string]gateway.Status{"payment.expired": gateway.StatusExpired, "payment.cancelled": gateway.StatusCancelled} {
		got, err := a.VerifyWebhook(hook("secretA-0123456789", now, body, ev, "d-"+ev), pathA, []byte(body))
		if err != nil || !got.StatusOK || got.Status != want {
			t.Errorf("%s: %+v %v", ev, got, err)
		}
	}
	got, err := a.VerifyWebhook(hook("secretA-0123456789", now, body, "payment.refunded", "d-x"), pathA, []byte(body))
	if err != nil || got.StatusOK {
		t.Errorf("unknown event: %+v %v; want unmapped", got, err)
	}
}

func TestTestWebhookUsesTheStoredSecretAndNeverLeaksIt(t *testing.T) {
	a, _ := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	steps := a.TestWebhook(context.Background(), merchantA)
	if len(steps) != 2 || !steps[0].OK || !steps[1].OK {
		t.Fatalf("configured merchant: %+v", steps)
	}
	for _, st := range steps {
		if strings.Contains(st.Message+st.Code, "secretA") {
			t.Fatalf("the secret leaked into a step: %+v", st)
		}
	}
	steps = a.TestWebhook(context.Background(), "44444444-4444-4444-4444-444444444444")
	if len(steps) != 1 || steps[0].OK || steps[0].Code != "no_credentials" {
		t.Fatalf("merchant without credentials: %+v", steps)
	}
	if steps = a.TestWebhook(context.Background(), "not-a-uuid"); steps[0].OK {
		t.Fatalf("bad merchant id accepted: %+v", steps)
	}
}

func TestRejectionNamesTheFailingCheckWithoutLeakingSecrets(t *testing.T) {
	a, _ := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {})
	body := []byte(`{"external_order_id":"` + payID + `"}`)
	pathA := "/webhooks/qrisgate/" + merchantA
	now := int64(1_700_000_000)
	reason := func(h http.Header, path string) string {
		_, err := a.VerifyWebhook(h, path, body)
		var se *gateway.SignatureError
		if !errors.As(err, &se) || !errors.Is(err, gateway.ErrInvalidSignature) {
			t.Fatalf("not a signature error: %v", err)
		}
		return se.Reason
	}
	good := hook("secretA-0123456789", now, string(body), "payment.paid", "d")
	if r := reason(http.Header{}, "/webhooks/qrisgate"); r != "no_merchant_id_in_path" {
		t.Errorf("path: %s", r)
	}
	if r := reason(http.Header{}, pathA); r != "missing_signature_header" {
		t.Errorf("no headers: %s", r)
	}
	noTS := good.Clone()
	noTS.Del("X-QRISGate-Timestamp")
	if r := reason(noTS, pathA); r != "missing_or_bad_timestamp_header" {
		t.Errorf("no timestamp: %s", r)
	}
	if r := reason(hook("secretA-0123456789", now-400, string(body), "payment.paid", "d"), pathA); r != "timestamp_skew:400s" {
		t.Errorf("skew: %s", r)
	}
	if r := reason(hook("secretA-0123456789", now, string(body), "payment.paid", "d"), "/webhooks/qrisgate/44444444-4444-4444-4444-444444444444"); r != "no_credentials_for_merchant" {
		t.Errorf("no credentials: %s", r)
	}
	if r := reason(hook("wrong-wrong-wrong-wrong", now, string(body), "payment.paid", "d"), pathA); r != "signature_mismatch" {
		t.Errorf("mismatch: %s", r)
	}
	for _, r := range []string{reason(good.Clone(), pathA+"x")} {
		if strings.Contains(r, "secretA") || strings.Contains(r, good.Get("X-QRISGate-Signature")) {
			t.Errorf("reason leaks a secret or signature: %s", r)
		}
	}
}
