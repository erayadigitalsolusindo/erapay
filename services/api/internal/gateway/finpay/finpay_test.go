package finpay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var now = time.Date(2026, 11, 18, 7, 0, 0, 0, time.UTC)

func newAdapter(t *testing.T, h http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a, err := New(Config{BaseURL: srv.URL, MerchantID: "M1", MerchantKey: "K1", CallbackURL: "https://erapay.test/webhooks/finpay", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func reply(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func create(a *Adapter, ttl time.Duration) (gateway.CreateResult, error) {
	return a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "pay-1", Method: "QRIS", Amount: 10000,
		Currency: "IDR", ExpiresAt: now.Add(ttl)})
}

func TestCreateSendsBasicAuthAndMinuteTimeout(t *testing.T) {
	var got map[string]any
	a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "M1" || p != "K1" || r.URL.Path != pathInitiate {
			t.Errorf("auth/path wrong: %v %v %v", u, p, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(w, 200, `{"responseCode":"2000000","stringQr":"000201"}`)
	})
	res, err := create(a, 30*time.Minute)
	if err != nil || res.QRPayload != "000201" || res.GatewayReference != "pay-1" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got["url"].(map[string]any)["callbackUrl"] != "https://erapay.test/webhooks/finpay" {
		t.Errorf("url = %v", got["url"])
	}
	o := got["order"].(map[string]any)
	if o["id"] != "pay-1" || o["amount"] != "10000" || o["timeout"] != float64(30) {
		t.Errorf("order = %v", o)
	}
	// Below FINPAY's minimum the timeout is raised, not rejected.
	if _, err := create(a, 2*time.Minute); err != nil || got["order"].(map[string]any)["timeout"] != float64(minTimeoutMin) {
		t.Errorf("short ttl: err=%v order=%v", err, got["order"])
	}
}

func TestCreateOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string // "ok", "rejected", "ambiguous"
	}{
		{"duplicate returns original QR", 409, `{"responseCode":"4090001","stringQr":"000201"}`, "ok"},
		{"duplicate without QR", 409, `{"responseCode":"4090001"}`, "ambiguous"},
		{"bad field", 400, `{"responseCode":"4000001","responseMessage":"Invalid Field Format"}`, "rejected"},
		{"bad key", 401, `{"responseCode":"4010000"}`, "rejected"},
		{"server error", 500, `{}`, "ambiguous"},
		{"timeout code", 504, `{}`, "ambiguous"},
		{"200 with other code", 200, `{"responseCode":"2020000"}`, "ambiguous"},
		{"200 without QR", 200, `{"responseCode":"2000000"}`, "ambiguous"},
		{"html error page", 404, `<html>`, "rejected"},
	}
	for _, c := range cases {
		a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, c.status, c.body) })
		_, err := create(a, time.Hour)
		got := "ok"
		switch {
		case gateway.IsAmbiguous(err):
			got = "ambiguous"
		case gateway.IsRejected(err):
			got = "rejected"
		case err != nil:
			got = err.Error()
		}
		if got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestCreateTransportFailureIsAmbiguous(t *testing.T) {
	a := newAdapter(t, func(http.ResponseWriter, *http.Request) {})
	a.cfg.BaseURL = "http://127.0.0.1:1" // nothing listens
	if _, err := create(a, time.Hour); !gateway.IsAmbiguous(err) {
		t.Fatalf("err = %v, want ambiguous", err)
	}
}

func TestCreateValidation(t *testing.T) {
	a := newAdapter(t, func(http.ResponseWriter, *http.Request) { t.Error("must not call gateway") })
	bad := []gateway.CreateRequest{
		{Reference: "", Method: "QRIS", Amount: 1, Currency: "IDR", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 0, Currency: "IDR", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 1, Currency: "USD", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 1, Currency: "IDR", ExpiresAt: now.Add(-time.Minute)},
	}
	for i, r := range bad {
		if _, err := a.CreatePayment(context.Background(), r); !gateway.IsRejected(err) {
			t.Errorf("case %d: err = %v", i, err)
		}
	}
	if _, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "p", Method: "VA"}); err != gateway.ErrNotSupported {
		t.Errorf("method VA: %v", err)
	}
}

func TestGetPayment(t *testing.T) {
	body := `{"responseCode":"2000000","data":{"order":{"amount":10000,"currency":"IDR"},"result":{"payment":{"status":"%s","amount":%s,"datetime":%s}}}}`
	run := func(status, amount, dt string) gateway.QueryResult {
		a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != pathCheck+"pay-1" {
				t.Errorf("path = %s", r.URL.Path)
			}
			reply(w, 200, fmt.Sprintf(body, status, amount, dt))
		})
		res, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "pay-1"})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if r := run("PENDING", "0", "null"); r.RawStatus != "PENDING" || r.Amount != 10000 || r.PaidAt != nil {
		t.Errorf("pending: %+v", r)
	}
	r := run("PAID", "10000", `"2026-11-18 14:05:00"`)
	if r.Amount != 10000 || r.PaidAt == nil || !r.PaidAt.Equal(time.Date(2026, 11, 18, 7, 5, 0, 0, time.UTC)) {
		t.Errorf("paid: %+v", r)
	}
	// A paid amount that differs from the order must be reported as paid, not as the order amount.
	if r := run("PAID", "9000", "null"); r.Amount != 9000 {
		t.Errorf("paid short: %+v", r)
	}
}

func TestGetPaymentErrors(t *testing.T) {
	a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, 404, `{"responseCode":"4040001"}`) })
	if _, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "x"}); !gateway.IsRejected(err) {
		t.Errorf("not found: %v", err)
	}
	if _, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "a/b"}); !gateway.IsRejected(err) {
		t.Errorf("path injection: %v", err)
	}
	b := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, 200, `{"responseCode":"2000000"}`) })
	if _, err := b.GetPayment(context.Background(), gateway.QueryRequest{Reference: "x"}); !gateway.IsAmbiguous(err) {
		t.Errorf("empty data: %v", err)
	}
}

func TestMapStatus(t *testing.T) {
	for raw, want := range map[string]gateway.Status{"PENDING": gateway.StatusPending, "PAID": gateway.StatusSucceeded,
		"EXPIRED": gateway.StatusExpired, "FAILURE": gateway.StatusFailed, "CANCELLED": gateway.StatusCancelled} {
		if got, ok := (&Adapter{}).MapStatus(raw); !ok || got != want {
			t.Errorf("%s: %v %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"CAPTURED", "REFUNDED", "AUTHORIZED", "", "paid"} {
		if _, ok := (&Adapter{}).MapStatus(raw); ok {
			t.Errorf("%q must stay unmapped", raw)
		}
	}
}

func TestConnection(t *testing.T) {
	for status, want := range map[int]bool{404: true, 401: false, 500: false} {
		a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, status, `{"responseCode":"x"}`) })
		if got := a.TestConnection(context.Background(), "", false)[0].OK; got != want {
			t.Errorf("status %d: ok=%v want %v", status, got, want)
		}
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	for _, c := range []Config{{BaseURL: SandboxURL}, {BaseURL: "nope", MerchantID: "a", MerchantKey: "b", CallbackURL: "https://x.test/cb"},
		{BaseURL: SandboxURL, MerchantID: "a", MerchantKey: "b"}, {BaseURL: SandboxURL, MerchantID: "a", MerchantKey: "b", CallbackURL: "http://x.test/cb"}} {
		if _, err := New(c); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
}
