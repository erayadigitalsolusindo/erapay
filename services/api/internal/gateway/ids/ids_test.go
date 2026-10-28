package ids

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const (
	testKey    = "ids_live_chan-uuid-1_s3cr3tvalue"
	testSecret = "callback-secret"
)

var fixedNow = time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)

func newTest(t *testing.T, h http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a, err := New(Config{BaseURL: srv.URL, APIKey: testKey, CallbackSecret: testSecret, Now: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNewValidation(t *testing.T) {
	for name, c := range map[string]Config{
		"bad key":      {BaseURL: "https://x.test", APIKey: "nope", CallbackSecret: testSecret},
		"short secret": {BaseURL: "https://x.test", APIKey: testKey, CallbackSecret: "short"},
		"bad url":      {BaseURL: "x.test", APIKey: testKey, CallbackSecret: testSecret},
	} {
		if _, err := New(c); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseAPIKey(t *testing.T) {
	ch, err := ParseAPIKey("ids_live_0190f1c0-aaaa_secret_with_underscores")
	if err != nil || ch != "0190f1c0-aaaa" {
		t.Fatalf("got %q, %v", ch, err)
	}
}

func TestCreatePaymentSendsIdempotentRequest(t *testing.T) {
	var got map[string]any
	var auth string
	a := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/v1/api/payments" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"code":201,"status":"success","payment":{"payment_id":"ids-pay-1","status":"PENDING","qr_string":"000201...","amount":250000}}`))
	})
	res, err := a.CreatePayment(context.Background(), gateway.CreateRequest{
		Reference: "erapay-pay-1", Method: "QRIS", Amount: 250000, Currency: "IDR", ExpiresAt: fixedNow.Add(15 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if res.GatewayReference != "ids-pay-1" || res.QRPayload != "000201..." || res.RawStatus != "PENDING" {
		t.Fatalf("result %+v", res)
	}
	if auth != "Bearer "+testKey {
		t.Errorf("auth header %q", auth)
	}
	if got["income_channel_transaction_id"] != "erapay-pay-1" || got["payment_method"] != "qris" ||
		got["income_channel_id"] != "chan-uuid-1" || got["currency"] != "IDR" {
		t.Errorf("payload %v", got)
	}
	if got["amount"] != float64(250000) || got["expiry_seconds"] != float64(900) {
		t.Errorf("amount/expiry %v %v", got["amount"], got["expiry_seconds"])
	}
}

func TestCreatePaymentClassification(t *testing.T) {
	cases := []struct {
		status    int
		body      string
		ambiguous bool
	}{
		{400, `{"code":400,"status":"error","message":"bad"}`, false},
		{401, `{"code":401,"message":"nope"}`, false},
		{403, `{}`, false},
		{409, `{}`, true},
		{429, `{}`, true},
		{502, `{"code":502}`, true},
		{500, ``, true},
		{201, `{"code":201,"payment":{"payment_id":"x"}}`, true}, // success without a QR: unknown what happened
	}
	for _, c := range cases {
		a := newTest(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		})
		_, err := a.CreatePayment(context.Background(), gateway.CreateRequest{
			Reference: "p1", Method: "QRIS", Amount: 1000, Currency: "IDR", ExpiresAt: fixedNow.Add(time.Minute)})
		if err == nil {
			t.Errorf("status %d: expected error", c.status)
			continue
		}
		if gateway.IsAmbiguous(err) != c.ambiguous || gateway.IsRejected(err) == c.ambiguous {
			t.Errorf("status %d: ambiguous=%v rejected=%v", c.status, gateway.IsAmbiguous(err), gateway.IsRejected(err))
		}
	}
}

func TestNonJSONErrorPointsAtBaseURL(t *testing.T) {
	a := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<!DOCTYPE html><html>not found</html>"))
	})
	steps := a.TestConnection(context.Background(), "", false)
	if len(steps) != 1 || steps[0].OK || !strings.Contains(steps[0].Message, "Base URL") {
		t.Fatalf("steps %+v", steps)
	}
}

func TestCreatePaymentTimeoutIsAmbiguous(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.CreatePayment(ctx, gateway.CreateRequest{Reference: "p1", Method: "QRIS", Amount: 1000, Currency: "IDR", ExpiresAt: fixedNow.Add(time.Minute)})
	if !gateway.IsAmbiguous(err) {
		t.Fatalf("want ambiguous, got %v", err)
	}
}

func TestCreatePaymentInvalidRequests(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) { t.Error("must not call the gateway") })
	base := gateway.CreateRequest{Reference: "p1", Method: "QRIS", Amount: 1000, Currency: "IDR", ExpiresAt: fixedNow.Add(time.Minute)}
	for name, mod := range map[string]func(*gateway.CreateRequest){
		"zero amount": func(r *gateway.CreateRequest) { r.Amount = 0 },
		"usd":         func(r *gateway.CreateRequest) { r.Currency = "USD" },
		"no ref":      func(r *gateway.CreateRequest) { r.Reference = "" },
		"expired":     func(r *gateway.CreateRequest) { r.ExpiresAt = fixedNow.Add(-time.Second) },
	} {
		r := base
		mod(&r)
		if _, err := a.CreatePayment(context.Background(), r); !gateway.IsRejected(err) {
			t.Errorf("%s: want rejected, got %v", name, err)
		}
	}
	r := base
	r.Method = "VA_BCA"
	if _, err := a.CreatePayment(context.Background(), r); err != gateway.ErrNotSupported {
		t.Errorf("want ErrNotSupported, got %v", err)
	}
}

func TestGetPaymentByOurReference(t *testing.T) {
	a := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("income_channel_transaction_id") != "erapay-pay-1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"code":200,"payment":{"status":"PAID","amount":250000,"paid_amount":250000,"currency":"IDR","paid_at":"2026-10-28T10:00:00Z"}}`))
	})
	res, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "erapay-pay-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RawStatus != "PAID" || res.Amount != 250000 || res.Currency != "IDR" || res.PaidAt == nil {
		t.Fatalf("result %+v", res)
	}
	if s, ok := a.MapStatus(res.RawStatus); !ok || s != gateway.StatusSucceeded {
		t.Errorf("map %v %v", s, ok)
	}
}

func TestGetPaymentNotFoundIsRejected(t *testing.T) {
	a := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
	if _, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "x"}); !gateway.IsRejected(err) {
		t.Fatalf("want rejected, got %v", err)
	}
}

func TestMapStatus(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) {})
	want := map[string]gateway.Status{"PENDING": gateway.StatusPending, "PAID": gateway.StatusSucceeded,
		"EXPIRED": gateway.StatusExpired, "FAILED": gateway.StatusFailed}
	for raw, w := range want {
		if s, ok := a.MapStatus(raw); !ok || s != w {
			t.Errorf("%s -> %v %v", raw, s, ok)
		}
	}
	for _, raw := range []string{"CREATING", "SETTLED", "", "paid"} {
		if _, ok := a.MapStatus(raw); ok {
			t.Errorf("%q must stay unmapped", raw)
		}
	}
}

func TestCancelNotSupported(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) {})
	if err := a.CancelPayment(context.Background(), gateway.CancelRequest{}); err != gateway.ErrNotSupported {
		t.Fatalf("got %v", err)
	}
	c := a.Capabilities()
	if c.Cancel || c.Refund || c.Disbursement || !c.Inquiry || !c.SupportsMethod("QRIS") {
		t.Fatalf("capabilities %+v", c)
	}
}

func TestTestConnection(t *testing.T) {
	ok := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/api/payment-methods" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{}`))
	})
	if steps := ok.TestConnection(context.Background(), "", false); len(steps) != 1 || !steps[0].OK {
		t.Fatalf("steps %+v", steps)
	}
	bad := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	if steps := bad.TestConnection(context.Background(), "", false); len(steps) != 1 || steps[0].OK || steps[0].Message == "" {
		t.Fatalf("steps %+v", steps)
	}
}

// ---- webhook ----

func signed(t *testing.T, ts time.Time, body string) http.Header {
	t.Helper()
	stamp := strconv.FormatInt(ts.Unix(), 10)
	h := http.Header{}
	h.Set("X-IDS-Timestamp", stamp)
	h.Set("X-IDS-Signature", CallbackSignature(testSecret, stamp, []byte(body)))
	return h
}

const paidBody = `{"payment_id":"ids-pay-1","income_channel_id":"chan-uuid-1","income_channel_transaction_id":"erapay-pay-1","status":"PAID","amount":250000,"paid_amount":250000,"currency":"IDR"}`

func TestVerifyWebhookPaid(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) {})
	ev, err := a.VerifyWebhook(signed(t, fixedNow, paidBody), "/webhooks/ids", []byte(paidBody))
	if err != nil {
		t.Fatal(err)
	}
	if ev.PaymentRef != "erapay-pay-1" || ev.Status != gateway.StatusSucceeded || !ev.StatusOK ||
		ev.Amount != 250000 || ev.Currency != "IDR" || ev.DedupKey != "ids:webhook:ids-pay-1:PAID" {
		t.Fatalf("event %+v", ev)
	}
}

func TestVerifyWebhookUsesPaidAmountOnMismatch(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) {})
	body := `{"payment_id":"p","income_channel_transaction_id":"x","status":"PAID","amount":250000,"paid_amount":249000}`
	ev, err := a.VerifyWebhook(signed(t, fixedNow, body), "/", []byte(body))
	if err != nil || ev.Amount != 249000 {
		t.Fatalf("%+v %v", ev, err)
	}
}

func TestVerifyWebhookRejectsBadAuth(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) {})
	body := []byte(paidBody)

	tampered := signed(t, fixedNow, paidBody)
	if _, err := a.VerifyWebhook(tampered, "/", []byte(paidBody+" ")); err != gateway.ErrInvalidSignature {
		t.Errorf("tampered body: %v", err)
	}
	if _, err := a.VerifyWebhook(signed(t, fixedNow.Add(-6*time.Minute), paidBody), "/", body); err != gateway.ErrInvalidSignature {
		t.Errorf("stale timestamp: %v", err)
	}
	if _, err := a.VerifyWebhook(signed(t, fixedNow.Add(6*time.Minute), paidBody), "/", body); err != gateway.ErrInvalidSignature {
		t.Errorf("future timestamp: %v", err)
	}
	if _, err := a.VerifyWebhook(http.Header{}, "/", body); err != gateway.ErrInvalidSignature {
		t.Errorf("missing headers: %v", err)
	}
	h := signed(t, fixedNow, paidBody)
	h.Set("X-IDS-Timestamp", "not-a-number")
	if _, err := a.VerifyWebhook(h, "/", body); err != gateway.ErrInvalidSignature {
		t.Errorf("bad timestamp: %v", err)
	}
	h = signed(t, fixedNow, paidBody)
	h.Set("X-IDS-Signature", "deadbeef")
	if _, err := a.VerifyWebhook(h, "/", body); err != gateway.ErrInvalidSignature {
		t.Errorf("bad signature: %v", err)
	}
}

func TestVerifyWebhookAuthenticButUnusable(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) {})
	for name, body := range map[string]string{
		"malformed":      `{not json`,
		"other channel":  `{"payment_id":"p","income_channel_id":"other","income_channel_transaction_id":"x","status":"PAID","amount":1}`,
		"no reference":   `{"payment_id":"p","status":"PAID","amount":1}`,
		"fractional amt": `{"payment_id":"p","income_channel_transaction_id":"x","status":"PENDING","amount":1.5}`,
	} {
		_, err := a.VerifyWebhook(signed(t, fixedNow, body), "/", []byte(body))
		if err == nil || err == gateway.ErrInvalidSignature {
			t.Errorf("%s: want a non-signature error, got %v", name, err)
		}
	}
}

func TestVerifyWebhookUnmappedStatusIsNotOK(t *testing.T) {
	a := newTest(t, func(http.ResponseWriter, *http.Request) {})
	body := `{"payment_id":"p","income_channel_transaction_id":"x","status":"SETTLED","amount":1000}`
	ev, err := a.VerifyWebhook(signed(t, fixedNow, body), "/", []byte(body))
	if err != nil || ev.StatusOK {
		t.Fatalf("%+v %v", ev, err)
	}
}
