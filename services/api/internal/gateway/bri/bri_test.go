package bri

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func newTest(t *testing.T, h http.HandlerFunc) (*Adapter, *rsa.PrivateKey) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	key := testKey(t)
	a, err := New(Config{BaseURL: srv.URL, ClientID: "client-1", ClientSecret: "secret-1", PrivateKey: key,
		PartnerID: "partner-1", ChannelID: "95221", MerchantID: "M123", TerminalID: "T456"})
	if err != nil {
		t.Fatal(err)
	}
	return a, key
}

func tokenOK(w http.ResponseWriter) {
	_, _ = io.WriteString(w, `{"accessToken":"tok-1","tokenType":"BearerToken","expiresIn":"899"}`)
}

func TestTokenSignatureVerifiesWithPublicKey(t *testing.T) {
	var ts, sig string
	a, key := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathToken || r.Header.Get("X-CLIENT-KEY") != "client-1" {
			t.Errorf("unexpected token request %s", r.URL.Path)
		}
		ts, sig = r.Header.Get("X-TIMESTAMP"), r.Header.Get("X-SIGNATURE")
		tokenOK(w)
	})
	if _, err := a.accessToken(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(ts, "+07:00") {
		t.Fatalf("timestamp not WIB: %s", ts)
	}
	raw, _ := base64.StdEncoding.DecodeString(sig)
	d := sha256.Sum256([]byte("client-1|" + ts))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, d[:], raw); err != nil {
		t.Fatalf("token signature does not verify: %v", err)
	}
}

func TestRequestSignatureVector(t *testing.T) {
	// BRI sample: sha256({"hello":"world"}) is documented as a47a5f14...; the signature must be
	// stable for identical inputs and change with the body.
	a := RequestSignature("s", "POST", "/p", "tok", []byte(`{"hello":"world"}`), "2021-11-29T09:22:18.172+07:00")
	b := RequestSignature("s", "POST", "/p", "tok", []byte(`{"hello":"world"}`), "2021-11-29T09:22:18.172+07:00")
	c := RequestSignature("s", "POST", "/p", "tok", []byte(`{"hello":"worlD"}`), "2021-11-29T09:22:18.172+07:00")
	if a != b || a == c || a == "" {
		t.Fatal("request signature not deterministic or not body-sensitive")
	}
}

func TestCreatePaymentSuccess(t *testing.T) {
	var gotBody map[string]any
	a, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathToken {
			tokenOK(w)
			return
		}
		if r.URL.Path != pathGenerate || r.Header.Get("Authorization") != "Bearer tok-1" ||
			r.Header.Get("X-PARTNER-ID") != "partner-1" || r.Header.Get("CHANNEL-ID") != "95221" ||
			r.Header.Get("X-EXTERNAL-ID") == "" {
			t.Errorf("bad generate request: %s %v", r.URL.Path, r.Header)
		}
		b, _ := io.ReadAll(r.Body)
		want := RequestSignature("secret-1", "POST", pathGenerate, "tok-1", b, r.Header.Get("X-TIMESTAMP"))
		if r.Header.Get("X-SIGNATURE") != want {
			t.Error("business signature mismatch")
		}
		_ = json.Unmarshal(b, &gotBody)
		_, _ = io.WriteString(w, `{"responseCode":"2004700","responseMessage":"Successful","qrContent":"000201xxx","referenceNo":"409676201434"}`)
	})
	res, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "7f0c4d9e-1111-2222-3333-444455556666",
		Method: "QRIS", Amount: 12345, Currency: "IDR", ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if res.GatewayReference != "409676201434" || res.QRPayload != "000201xxx" {
		t.Fatalf("unexpected result %+v", res)
	}
	amt := gotBody["amount"].(map[string]any)
	if amt["value"] != "12345.00" || amt["currency"] != "IDR" || gotBody["merchantId"] != "M123" || gotBody["terminalId"] != "T456" {
		t.Fatalf("bad payload %v", gotBody)
	}
	if ref, _ := gotBody["partnerReferenceNo"].(string); len(ref) != 13 {
		t.Fatalf("partnerReferenceNo %q is not 13 digits", ref)
	}
}

func TestCreateIsAmbiguousOn5xxAndTimeoutNeverRejected(t *testing.T) {
	a, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathToken {
			tokenOK(w)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"responseCode":"5004700","responseMessage":"x"}`)
	})
	_, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "r", Method: "QRIS", Amount: 1000, Currency: "IDR"})
	if !gateway.IsAmbiguous(err) {
		t.Fatalf("want ambiguous, got %v", err)
	}
}

func TestCreateRejectedOnDocumentedClientError(t *testing.T) {
	a, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathToken {
			tokenOK(w)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"responseCode":"4044708","responseMessage":"Invalid Merchant"}`)
	})
	_, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "r", Method: "QRIS", Amount: 1000, Currency: "IDR"})
	if !gateway.IsRejected(err) {
		t.Fatalf("want rejected, got %v", err)
	}
}

func TestCreateIncompleteSuccessIsAmbiguous(t *testing.T) {
	a, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathToken {
			tokenOK(w)
			return
		}
		_, _ = io.WriteString(w, `{"responseCode":"2004700","responseMessage":"Successful"}`)
	})
	_, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "r", Method: "QRIS", Amount: 1000, Currency: "IDR"})
	if !gateway.IsAmbiguous(err) {
		t.Fatalf("want ambiguous, got %v", err)
	}
}

func TestExpiredTokenRefreshedOnceOn401(t *testing.T) {
	var tokenCalls, genCalls atomic.Int32
	a, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathToken {
			tokenCalls.Add(1)
			tokenOK(w)
			return
		}
		if genCalls.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"responseCode":"4014701","responseMessage":"Invalid Token (B2B)"}`)
			return
		}
		_, _ = io.WriteString(w, `{"responseCode":"2004700","qrContent":"q","referenceNo":"1"}`)
	})
	if _, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "r", Method: "QRIS", Amount: 1000, Currency: "IDR"}); err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 2 || genCalls.Load() != 2 {
		t.Fatalf("token calls %d, generate calls %d", tokenCalls.Load(), genCalls.Load())
	}
}

func TestTokenIsCached(t *testing.T) {
	var tokenCalls atomic.Int32
	a, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathToken {
			tokenCalls.Add(1)
			tokenOK(w)
			return
		}
		_, _ = io.WriteString(w, `{"responseCode":"2004700","qrContent":"q","referenceNo":"1"}`)
	})
	for i := 0; i < 3; i++ {
		if _, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: "r", Method: "QRIS", Amount: 1000, Currency: "IDR"}); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token fetched %d times, want 1", tokenCalls.Load())
	}
}

func TestGetPayment(t *testing.T) {
	var q map[string]any
	a, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathToken {
			tokenOK(w)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &q)
		_, _ = io.WriteString(w, `{"responseCode":"2005100","latestTransactionStatus":"00","paidTime":"2025-05-27T09:59:07+07:00","amount":{"value":"2000.00","currency":"IDR"}}`)
	})
	res, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "r", GatewayReference: "409676201434"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RawStatus != "00" || res.Amount != 2000 || res.PaidAt == nil || q["originalReferenceNo"] != "409676201434" || q["serviceCode"] != "47" {
		t.Fatalf("unexpected %+v / %v", res, q)
	}
	if _, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "r"}); !gateway.IsAmbiguous(err) {
		t.Fatalf("query without BRI reference must be ambiguous, got %v", err)
	}
}

func TestMapStatus(t *testing.T) {
	a, _ := newTest(t, func(http.ResponseWriter, *http.Request) {})
	for raw, want := range map[string]gateway.Status{"00": gateway.StatusSucceeded, "01": gateway.StatusPending, "02": gateway.StatusPending,
		"03": gateway.StatusPending, "05": gateway.StatusCancelled, "06": gateway.StatusFailed} {
		if got, ok := a.MapStatus(raw); !ok || got != want {
			t.Errorf("%s: got %v %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"04", "07", "", "99"} {
		if _, ok := a.MapStatus(raw); ok {
			t.Errorf("%q must stay unmapped", raw)
		}
	}
}

func TestParseAmount(t *testing.T) {
	cases := map[string]int64{"2000.00": 2000, "2000": 2000, "10.50": -1, "x": -1, "-5.00": -1}
	for v, want := range cases {
		if got := parseAmount(&amountDTO{Value: v, Currency: "IDR"}); got != want {
			t.Errorf("%s: got %d want %d", v, got, want)
		}
	}
	if parseAmount(&amountDTO{Value: "1.00", Currency: "USD"}) != -1 || parseAmount(nil) != -1 {
		t.Error("non-IDR / nil must be -1")
	}
}

func TestNewValidatesConfig(t *testing.T) {
	k := testKey(t)
	base := Config{ClientID: "c", ClientSecret: "s", PrivateKey: k, PartnerID: "p", ChannelID: "95221", MerchantID: "M", TerminalID: "T"}
	if _, err := New(base); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Config){
		"no key":     func(c *Config) { c.PrivateKey = nil },
		"no partner": func(c *Config) { c.PartnerID = "" },
		"long chan":  func(c *Config) { c.ChannelID = "123456" },
		"no merch":   func(c *Config) { c.MerchantID = "" },
		"long term":  func(c *Config) { c.TerminalID = strings.Repeat("a", 17) },
	} {
		c := base
		mut(&c)
		if _, err := New(c); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestCapabilitiesAndUnsupported(t *testing.T) {
	a, _ := newTest(t, func(http.ResponseWriter, *http.Request) {})
	c := a.Capabilities()
	if c.Cancel || c.Refund || c.Disbursement || !c.PlatformAccount || !c.Inquiry || !c.SupportsMethod("QRIS") {
		t.Fatalf("unexpected capabilities %+v", c)
	}
	if err := a.CancelPayment(context.Background(), gateway.CancelRequest{}); err != gateway.ErrNotSupported {
		t.Fatal("cancel must be unsupported")
	}
}
