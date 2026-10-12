package doku

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// Shape taken from a real sandbox QRIS notification; every value here is invented.
const sampleBody = `{
  "service": {"id": "QRIS", "name": "QRIS"},
  "customer": {"name": "PAYER NAME", "email": "payer@example.test", "phone": "628000000000"},
  "order": {"invoice_number": "c0282b1c-59e8-4b9c-ad0a-3a7cb0c3502c", "amount": 1000.00, "tips": 0.00, "terminal_id": "TERM01        "},
  "transaction": {"status": "SUCCESS", "date": "2026-10-12T06:25:24Z", "original_request_id": "123"}
}`

const hookPath = "/webhooks/doku"

func signed(a *Adapter, body, path string) http.Header {
	h := http.Header{}
	h.Set("Client-Id", clientID)
	h.Set("Request-Id", "req-1")
	h.Set("Request-Timestamp", "2026-10-12T06:25:24Z")
	h.Set("Signature", WebhookSignature(a.cfg.ClientSecret, clientID, "req-1", "2026-10-12T06:25:24Z", path, []byte(body)))
	return h
}

func TestVerifyWebhookAcceptsValidSignatureAndParses(t *testing.T) {
	_, a := newFake(t)
	ev, err := a.VerifyWebhook(signed(a, sampleBody, hookPath), hookPath, []byte(sampleBody))
	if err != nil {
		t.Fatal(err)
	}
	if ev.PaymentRef != "c0282b1c-59e8-4b9c-ad0a-3a7cb0c3502c" || ev.Amount != 1000 || ev.Currency != "IDR" ||
		!ev.StatusOK || ev.Status != gateway.StatusSucceeded || ev.RawStatus != "SUCCESS" || ev.DedupKey != "doku:webhook:req-1" {
		t.Fatalf("%+v", ev)
	}
}

func TestVerifyWebhookRejectsAnythingNotAuthentic(t *testing.T) {
	_, a := newFake(t)
	body := []byte(sampleBody)

	cases := map[string]func(h http.Header) (http.Header, string, []byte){
		"tampered body": func(h http.Header) (http.Header, string, []byte) {
			return h, hookPath, []byte(strings.Replace(sampleBody, "1000.00", "9000.00", 1))
		},
		"other path": func(h http.Header) (http.Header, string, []byte) { return h, "/webhooks/doku/x", body },
		"wrong client id": func(h http.Header) (http.Header, string, []byte) {
			h.Set("Client-Id", "someone-else")
			return h, hookPath, body
		},
		"bad signature": func(h http.Header) (http.Header, string, []byte) {
			h.Set("Signature", "HMACSHA256=AAAA")
			return h, hookPath, body
		},
		"missing signature":  func(h http.Header) (http.Header, string, []byte) { h.Del("Signature"); return h, hookPath, body },
		"missing request id": func(h http.Header) (http.Header, string, []byte) { h.Del("Request-Id"); return h, hookPath, body },
		"changed timestamp": func(h http.Header) (http.Header, string, []byte) {
			h.Set("Request-Timestamp", "2026-10-12T06:25:25Z")
			return h, hookPath, body
		},
		"changed request id": func(h http.Header) (http.Header, string, []byte) {
			h.Set("Request-Id", "req-2")
			return h, hookPath, body
		},
	}
	for name, mutate := range cases {
		h, p, b := mutate(signed(a, sampleBody, hookPath))
		if _, err := a.VerifyWebhook(h, p, b); !errors.Is(err, gateway.ErrInvalidSignature) {
			t.Errorf("%s: want ErrInvalidSignature, got %v", name, err)
		}
	}
}

func TestVerifyWebhookAuthenticButUnusablePayload(t *testing.T) {
	_, a := newFake(t)
	for name, body := range map[string]string{
		"fractional amount": strings.Replace(sampleBody, "1000.00", "1000.50", 1),
		"exponent amount":   strings.Replace(sampleBody, "1000.00", "1e3", 1),
		"negative amount":   strings.Replace(sampleBody, "1000.00", "-1000.00", 1),
		"no invoice":        strings.Replace(sampleBody, "c0282b1c-59e8-4b9c-ad0a-3a7cb0c3502c", "", 1),
		"not json":          "oops",
	} {
		_, err := a.VerifyWebhook(signed(a, body, hookPath), hookPath, []byte(body))
		if err == nil || errors.Is(err, gateway.ErrInvalidSignature) {
			t.Errorf("%s: want a parse error, got %v", name, err)
		}
	}
}

func TestVerifyWebhookUnobservedStatusIsNotApplicable(t *testing.T) {
	_, a := newFake(t)
	body := strings.Replace(sampleBody, `"SUCCESS"`, `"FAILED"`, 1)
	ev, err := a.VerifyWebhook(signed(a, body, hookPath), hookPath, []byte(body))
	if err != nil || ev.StatusOK {
		t.Fatalf("%+v %v", ev, err)
	}
}

func TestWholeAmount(t *testing.T) {
	for in, want := range map[string]int64{"1000.00": 1000, "1000": 1000, "0": 0, "12500.0": 12500} {
		if got, err := wholeAmount(in); err != nil || got != want {
			t.Errorf("%q: %d %v", in, got, err)
		}
	}
	for _, in := range []string{"", "1000.01", "1e3", "-5", "+5", "abc", "99999999999999999999"} {
		if _, err := wholeAmount(in); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}
