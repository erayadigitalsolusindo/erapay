package xendit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

var now = time.Date(2026, 11, 19, 7, 0, 0, 0, time.UTC)

func newAdapter(t *testing.T, h http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a, err := New(Config{BaseURL: srv.URL, SecretKey: "xnd_test", Now: func() time.Time { return now }})
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

func create(a *Adapter) (gateway.CreateResult, error) {
	return a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "pay-1", Method: "QRIS", Amount: 10000,
		Currency: "IDR", ExpiresAt: now.Add(30 * time.Minute)})
}

func TestCreateSendsAuthAndBody(t *testing.T) {
	var got map[string]any
	a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "xnd_test" || p != "" || r.URL.Path != pathRequest || r.Header.Get("api-version") != apiVersion {
			t.Errorf("auth/path/version wrong: %v %q %v", u, p, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(w, 201, `{"payment_request_id":"pr-1","status":"REQUIRES_ACTION","actions":[{"type":"PRESENT_TO_CUSTOMER","descriptor":"QR_STRING","value":"000201"}]}`)
	})
	res, err := create(a)
	if err != nil || res.QRPayload != "000201" || res.GatewayReference != "pr-1" || res.RawStatus != "REQUIRES_ACTION" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got["reference_id"] != "pay-1" || got["request_amount"] != float64(10000) || got["channel_code"] != "QRIS" ||
		got["channel_properties"].(map[string]any)["expires_at"] != "2026-11-19T07:30:00Z" {
		t.Errorf("body = %v", got)
	}
}

func TestCreateOutcomes(t *testing.T) {
	for _, c := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"ok", 201, `{"payment_request_id":"pr-1","actions":[{"descriptor":"QR_STRING","value":"x"}]}`, "ok"},
		{"no QR", 201, `{"payment_request_id":"pr-1"}`, "ambiguous"},
		{"validation", 400, `{"error_code":"API_VALIDATION_ERROR","message":"bad"}`, "rejected"},
		{"bad key", 401, `{}`, "rejected"},
		{"forbidden", 403, `{"error_code":"INVALID_MERCHANT_SETTINGS"}`, "rejected"},
		{"duplicate", 409, `{"error_code":"DUPLICATE_ERROR"}`, "ambiguous"},
		{"rate limit", 429, `{}`, "ambiguous"},
		{"server", 500, `{}`, "ambiguous"},
		{"channel unavailable", 503, `{"error_code":"CHANNEL_UNAVAILABLE"}`, "ambiguous"},
	} {
		a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, c.status, c.body) })
		_, err := create(a)
		got := "ok"
		switch {
		case gateway.IsAmbiguous(err):
			got = "ambiguous"
		case gateway.IsRejected(err):
			got = "rejected"
		case err != nil:
			got = "other"
		}
		if got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestCreateTransportFailureIsAmbiguous(t *testing.T) {
	a := newAdapter(t, func(http.ResponseWriter, *http.Request) {})
	a.cfg.BaseURL = "http://127.0.0.1:1"
	if _, err := create(a); !gateway.IsAmbiguous(err) {
		t.Errorf("err = %v", err)
	}
}

func TestCreateValidation(t *testing.T) {
	a := newAdapter(t, func(http.ResponseWriter, *http.Request) { t.Error("must not call gateway") })
	for _, r := range []gateway.CreateRequest{
		{Reference: "", Method: "QRIS", Amount: 1, Currency: "IDR", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 0, Currency: "IDR", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 1, Currency: "USD", ExpiresAt: now.Add(time.Hour)},
		{Reference: "p", Method: "QRIS", Amount: 1, Currency: "IDR", ExpiresAt: now.Add(-time.Hour)},
	} {
		if _, err := a.CreatePayment(context.Background(), r); !gateway.IsRejected(err) {
			t.Errorf("%+v: err = %v", r, err)
		}
	}
	if _, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Method: "VA"}); err != gateway.ErrNotSupported {
		t.Errorf("method: %v", err)
	}
}

func TestGetPayment(t *testing.T) {
	a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathRequest+"/pr-1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		reply(w, 200, `{"status":"SUCCEEDED","request_amount":10000,"currency":"IDR","updated":"2026-11-19T07:05:00Z"}`)
	})
	res, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "pay-1", GatewayReference: "pr-1"})
	if err != nil || res.RawStatus != "SUCCEEDED" || res.Amount != 10000 || res.PaidAt == nil || !res.PaidAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	// Pending: no paid time.
	b := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 200, `{"status":"REQUIRES_ACTION","request_amount":10000,"currency":"IDR","updated":"2026-11-19T07:05:00Z"}`)
	})
	if res, err := b.GetPayment(context.Background(), gateway.QueryRequest{GatewayReference: "pr-1"}); err != nil || res.PaidAt != nil {
		t.Errorf("pending: %+v %v", res, err)
	}
}

func TestGetPaymentErrors(t *testing.T) {
	a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, 404, `{"error_code":"DATA_NOT_FOUND"}`) })
	if _, err := a.GetPayment(context.Background(), gateway.QueryRequest{}); !gateway.IsAmbiguous(err) {
		t.Errorf("no id: %v", err)
	}
	if _, err := a.GetPayment(context.Background(), gateway.QueryRequest{GatewayReference: "a/b"}); !gateway.IsRejected(err) {
		t.Errorf("bad id: %v", err)
	}
	if _, err := a.GetPayment(context.Background(), gateway.QueryRequest{GatewayReference: "pr-1"}); !gateway.IsRejected(err) {
		t.Errorf("404: %v", err)
	}
}

func TestMapStatus(t *testing.T) {
	a := newAdapter(t, nil)
	for raw, want := range map[string]gateway.Status{"REQUIRES_ACTION": gateway.StatusPending, "SUCCEEDED": gateway.StatusSucceeded,
		"EXPIRED": gateway.StatusExpired, "FAILED": gateway.StatusFailed, "CANCELED": gateway.StatusCancelled} {
		if got, ok := a.MapStatus(raw); !ok || got != want {
			t.Errorf("%s: %v %v", raw, got, ok)
		}
	}
	if _, ok := a.MapStatus("AUTHORIZED"); ok {
		t.Error("AUTHORIZED must stay unmapped")
	}
}

func TestConnection(t *testing.T) {
	for status, want := range map[int]bool{404: true, 401: false, 500: false} {
		a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, status, `{}`) })
		if st := a.TestConnection(context.Background(), "", false); len(st) != 1 || st[0].OK != want {
			t.Errorf("%d: %+v", status, st)
		}
	}
}
