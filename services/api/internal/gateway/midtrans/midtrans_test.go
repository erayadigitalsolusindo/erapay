package midtrans

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var now = time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)

const sigKey = "Mid-server-signature-test"

func newAdapter(t *testing.T, h http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a, err := New(Config{BaseURL: srv.URL, ServerKey: "SB-Mid-server-test", SignatureKey: sigKey, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// reply mimics Midtrans: HTTP 200 whatever the body status_code says.
func reply(w http.ResponseWriter, httpStatus int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_, _ = io.WriteString(w, body)
}

func create(a *Adapter) (gateway.CreateResult, error) {
	return a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "pay-1", Method: "QRIS", Amount: 10000,
		Currency: "IDR", ExpiresAt: now.Add(30*time.Minute + time.Second)})
}

func TestCreateSendsAuthAndBody(t *testing.T) {
	var got map[string]any
	a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "SB-Mid-server-test" || p != "" || r.URL.Path != "/v2/charge" || r.Method != http.MethodPost {
			t.Errorf("auth/path wrong: %v %q %v", u, p, r.URL.Path)
		}
		if r.Header.Get("Idempotency-Key") != "pay-1" {
			t.Errorf("idempotency key = %q", r.Header.Get("Idempotency-Key"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(w, 200, `{"status_code":"201","transaction_id":"tx-1","transaction_status":"pending","qr_string":"000201"}`)
	})
	res, err := create(a)
	if err != nil || res.QRPayload != "000201" || res.GatewayReference != "tx-1" || res.RawStatus != "pending" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	td := got["transaction_details"].(map[string]any)
	ce := got["custom_expiry"].(map[string]any)
	if got["payment_type"] != "qris" || td["order_id"] != "pay-1" || td["gross_amount"] != float64(10000) ||
		ce["expiry_duration"] != float64(31) || ce["unit"] != "minute" { // 30m1s rounds up to 31
		t.Errorf("body = %v", got)
	}
}

func TestCreateOutcomes(t *testing.T) {
	for _, c := range []struct {
		name       string
		httpStatus int
		body       string
		ambiguous  bool
		wantOK     bool
	}{
		{"created", 200, `{"status_code":"201","transaction_id":"tx","qr_string":"q"}`, false, true},
		{"duplicate order id is a body 406 behind HTTP 200", 200, `{"status_code":"406","status_message":"conflict"}`, true, false},
		{"bad key is a body 401 behind HTTP 200", 200, `{"status_code":"401","status_message":"x"}`, false, false},
		{"validation 400", 200, `{"status_code":"400","status_message":"x"}`, false, false},
		{"rate limit", 200, `{"status_code":"429"}`, true, false},
		{"body 500", 200, `{"status_code":"500"}`, true, false},
		{"http 503", 503, `oops`, true, false},
		{"processing 202", 202, `{"status_code":"202"}`, true, false},
		{"success without qr is ambiguous", 200, `{"status_code":"201","transaction_id":"tx"}`, true, false},
		{"200 not JSON", 200, `<html>`, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, c.httpStatus, c.body) })
			_, err := create(a)
			if (err == nil) != c.wantOK || (err != nil && gateway.IsAmbiguous(err) != c.ambiguous) {
				t.Fatalf("err=%v ambiguous=%v", err, gateway.IsAmbiguous(err))
			}
		})
	}
}

func TestCreateTransportFailureIsAmbiguous(t *testing.T) {
	a := newAdapter(t, func(http.ResponseWriter, *http.Request) {})
	a.cfg.BaseURL = "http://127.0.0.1:1"
	if _, err := create(a); !gateway.IsAmbiguous(err) {
		t.Fatalf("err=%v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	a := newAdapter(t, func(http.ResponseWriter, *http.Request) { t.Error("must not call the gateway") })
	for _, r := range []gateway.CreateRequest{
		{Reference: "bad id", Method: "QRIS", Amount: 1, Currency: "IDR", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 0, Currency: "IDR", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 1, Currency: "USD", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 1, Currency: "IDR", ExpiresAt: now.Add(-time.Minute)},
	} {
		if _, err := a.CreatePayment(context.Background(), r); !gateway.IsRejected(err) {
			t.Errorf("%+v: err=%v", r, err)
		}
	}
	if _, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Method: "VA"}); !errors.Is(err, gateway.ErrNotSupported) {
		t.Errorf("VA: %v", err)
	}
}

func TestGetPayment(t *testing.T) {
	for _, c := range []struct {
		name, body, raw string
		amount          int64
		paid            bool
		err             string // "", "ambiguous", "rejected"
	}{
		{"expired 407", `{"status_code":"407","transaction_status":"expire","gross_amount":"10000.00","currency":"IDR"}`, "expire", 10000, false, ""},
		{"denied 202", `{"status_code":"202","transaction_status":"deny","gross_amount":"10000.00","currency":"IDR"}`, "deny", 10000, false, ""},
		{"pending", `{"status_code":"201","transaction_status":"pending","gross_amount":"10000.00","currency":"IDR","fraud_status":"accept"}`, "pending", 10000, false, ""},
		{"settled", `{"status_code":"200","transaction_status":"settlement","gross_amount":"10000.00","currency":"IDR","fraud_status":"accept","settlement_time":"2026-11-19 14:05:00"}`, "settlement", 10000, true, ""},
		{"settled but fraud challenge stays unmapped", `{"status_code":"200","transaction_status":"settlement","gross_amount":"10000.00","fraud_status":"challenge"}`, "fraud-challenge", 10000, false, ""},
		{"fractional amount is not trusted", `{"status_code":"200","transaction_status":"pending","gross_amount":"10000.50"}`, "pending", -1, false, ""},
		{"unknown order stays ambiguous", `{"status_code":"404","status_message":"Transaction doesn't exist."}`, "", 0, false, "ambiguous"},
		{"bad key", `{"status_code":"401"}`, "", 0, false, "rejected"},
		{"no status", `{"status_code":"200"}`, "", 0, false, "ambiguous"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/pay-1/status" || r.Method != http.MethodGet {
					t.Errorf("path=%s", r.URL.Path)
				}
				reply(w, 200, c.body)
			})
			q, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "pay-1"})
			switch c.err {
			case "ambiguous":
				if !gateway.IsAmbiguous(err) {
					t.Fatalf("err=%v", err)
				}
				return
			case "rejected":
				if !gateway.IsRejected(err) {
					t.Fatalf("err=%v", err)
				}
				return
			}
			if err != nil || q.RawStatus != c.raw || q.Amount != c.amount || (q.PaidAt != nil) != c.paid {
				t.Fatalf("q=%+v err=%v", q, err)
			}
			if c.paid && !q.PaidAt.Equal(time.Date(2026, 11, 19, 7, 5, 0, 0, time.UTC)) { // WIB is UTC+7
				t.Errorf("paidAt=%v", q.PaidAt)
			}
		})
	}
}

func TestMapStatus(t *testing.T) {
	a := newAdapter(t, nil)
	for raw, want := range map[string]gateway.Status{"pending": gateway.StatusPending, "settlement": gateway.StatusSucceeded,
		"expire": gateway.StatusExpired, "cancel": gateway.StatusCancelled, "deny": gateway.StatusFailed, "failure": gateway.StatusFailed} {
		if got, ok := a.MapStatus(raw); !ok || got != want {
			t.Errorf("%s: %v %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"capture", "authorize", "refund", "partial_refund", "chargeback", "", "SETTLEMENT"} {
		if _, ok := a.MapStatus(raw); ok {
			t.Errorf("%q must stay unmapped", raw)
		}
	}
}

func TestTestConnection(t *testing.T) {
	for _, c := range []struct {
		body string
		ok   bool
	}{
		{`{"status_code":"404","status_message":"Transaction doesn't exist."}`, true},
		{`{"status_code":"401","status_message":"unauthorized"}`, false},
	} {
		a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, 200, c.body) })
		if st := a.TestConnection(context.Background(), "", false)[0]; st.OK != c.ok {
			t.Errorf("%s: %+v", c.body, st)
		}
	}
}

// signed builds a notification the way Midtrans does: signature over order_id, status_code, gross_amount.
func signed(order, tx, code, status, amount, fraud string) []byte {
	b, _ := json.Marshal(map[string]string{"order_id": order, "transaction_id": tx, "status_code": code,
		"transaction_status": status, "gross_amount": amount, "currency": "IDR", "fraud_status": fraud,
		"signature_key": Signature(order, code, amount, sigKey)})
	return b
}

func TestSignatureShape(t *testing.T) {
	// The real formula was confirmed against sandbox responses with the real secret (not stored here).
	if got := Signature("a", "200", "10000.00", "k"); len(got) != 128 || got != Signature("a", "200", "10000.00", "k") {
		t.Fatal("signature must be 128 hex chars and deterministic")
	}
	if Signature("a", "200", "10000.00", "k") == Signature("a", "200", "10000", "k") {
		t.Fatal("gross_amount must be used exactly as spelled")
	}
}

func TestVerifyWebhook(t *testing.T) {
	a := newAdapter(t, nil)
	ev, err := a.VerifyWebhook(nil, "/webhooks/midtrans", signed("pay-1", "tx-1", "200", "settlement", "10000.00", "accept"))
	if err != nil || ev.PaymentRef != "pay-1" || !ev.StatusOK || ev.Status != gateway.StatusSucceeded || ev.Amount != 10000 ||
		ev.DedupKey != "midtrans:webhook:tx-1:settlement" {
		t.Fatalf("ev=%+v err=%v", ev, err)
	}
	for name, c := range map[string]struct {
		body []byte
		st   gateway.Status
		ok   bool
	}{
		"pending":                {signed("pay-1", "tx-1", "201", "pending", "10000.00", "accept"), gateway.StatusPending, true},
		"expire":                 {signed("pay-1", "tx-1", "202", "expire", "10000.00", ""), gateway.StatusExpired, true},
		"cancel":                 {signed("pay-1", "tx-1", "202", "cancel", "10000.00", ""), gateway.StatusCancelled, true},
		"settlement under 202":   {signed("pay-1", "tx-1", "202", "settlement", "10000.00", ""), "", false},
		"settlement fraud deny":  {signed("pay-1", "tx-1", "200", "settlement", "10000.00", "deny"), "", false},
		"refund is not a status": {signed("pay-1", "tx-1", "200", "refund", "10000.00", "accept"), "", false},
	} {
		got, err := a.VerifyWebhook(nil, "", c.body)
		if err != nil || got.StatusOK != c.ok || (c.ok && got.Status != c.st) {
			t.Errorf("%s: %+v %v", name, got, err)
		}
	}
}

func TestVerifyWebhookRejects(t *testing.T) {
	a := newAdapter(t, nil)
	good := signed("pay-1", "tx-1", "200", "settlement", "10000.00", "accept")
	var m map[string]string
	_ = json.Unmarshal(good, &m)
	tamper := func(k, v string) []byte {
		c := map[string]string{}
		for a, b := range m {
			c[a] = b
		}
		c[k] = v
		b, _ := json.Marshal(c)
		return b
	}
	for name, body := range map[string]([]byte){
		"amount changed":      tamper("gross_amount", "99999.00"),
		"order changed":       tamper("order_id", "pay-2"),
		"status_code changed": tamper("status_code", "201"),
		"no signature":        tamper("signature_key", ""),
		"wrong signature":     tamper("signature_key", Signature("pay-1", "200", "10000.00", "other")),
		"not json":            []byte("nope"),
	} {
		if _, err := a.VerifyWebhook(nil, "", body); !errors.Is(err, gateway.ErrInvalidSignature) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	// Without the signature secret nothing is accepted, even a body signed with an empty key.
	b, _ := New(Config{ServerKey: "k"})
	if _, err := b.VerifyWebhook(nil, "", signed("pay-1", "tx-1", "200", "settlement", "10000.00", "accept")); !errors.Is(err, gateway.ErrInvalidSignature) {
		t.Errorf("no signature key: %v", err)
	}
	// Authentic but unusable: fractional rupiah is not rounded.
	if _, err := a.VerifyWebhook(nil, "", signed("pay-1", "tx-1", "200", "settlement", "10000.50", "accept")); err == nil || errors.Is(err, gateway.ErrInvalidSignature) {
		t.Errorf("fraction: %v", err)
	}
}
