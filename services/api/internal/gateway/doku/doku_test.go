package doku

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// fakeDOKU is a local stand-in that VERIFIES signatures independently of the adapter.
// It proves the adapter follows doku.md; it is not a substitute for the real sandbox.
type fakeDOKU struct {
	t       *testing.T
	pub     *rsa.PublicKey
	secret  string
	srv     *httptest.Server
	tokens  atomic.Int32
	calls   atomic.Int32
	mu      sync.Mutex
	seenIDs map[string]bool
	last    map[string]any // last decoded transactional body
	lastRaw []byte
	// behaviour
	respond  func(path string, body map[string]any) (int, any)
	expire   atomic.Bool // when set, the next transactional call gets 401 once
	tokenTTL int
}

const clientID = "client-123"

func newFake(t *testing.T) (*fakeDOKU, *Adapter) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeDOKU{t: t, pub: &key.PublicKey, secret: "s3cret", seenIDs: map[string]bool{}, tokenTTL: 900}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	a, err := New(Config{BaseURL: f.srv.URL, ClientID: clientID, ClientSecret: f.secret, PrivateKey: key,
		TerminalID: "TERM01", PostalCode: "12345", HTTP: &http.Client{Timeout: 2 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	return f, a
}

func (f *fakeDOKU) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	if r.URL.Path == pathToken {
		f.tokens.Add(1)
		d := sha256.Sum256([]byte(clientID + "|" + r.Header.Get("X-TIMESTAMP")))
		sig, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-SIGNATURE"))
		if r.Header.Get("X-CLIENT-KEY") != clientID || rsa.VerifyPKCS1v15(f.pub, crypto.SHA256, d[:], sig) != nil ||
			string(body) != `{"grantType":"client_credentials"}` {
			w.WriteHeader(401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "tok-" + r.Header.Get("X-TIMESTAMP"),
			"tokenType": "Bearer", "expiresIn": f.tokenTTL})
		return
	}
	f.calls.Add(1)
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	// Independent recomputation of the symmetric signature from the bytes received.
	want := RequestSignature(f.secret, r.Method, r.URL.Path, tok, body, r.Header.Get("X-TIMESTAMP"))
	if r.Header.Get("X-SIGNATURE") != want {
		f.t.Errorf("bad request signature on %s", r.URL.Path)
		w.WriteHeader(401)
		return
	}
	for h, v := range map[string]string{"X-PARTNER-ID": clientID, "CHANNEL-ID": "H2H"} {
		if r.Header.Get(h) != v {
			f.t.Errorf("header %s = %q", h, r.Header.Get(h))
		}
	}
	id := r.Header.Get("X-EXTERNAL-ID")
	if !regexp.MustCompile(`^\d+$`).MatchString(id) {
		f.t.Errorf("X-EXTERNAL-ID not numeric: %q", id)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`).MatchString(r.Header.Get("X-TIMESTAMP")) {
		f.t.Errorf("bad X-TIMESTAMP %q", r.Header.Get("X-TIMESTAMP"))
	}
	f.mu.Lock()
	if f.seenIDs[id] {
		f.t.Errorf("X-EXTERNAL-ID reused: %s", id)
	}
	f.seenIDs[id] = true
	f.mu.Unlock()
	if f.expire.CompareAndSwap(true, false) {
		w.WriteHeader(401)
		return
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	f.mu.Lock()
	f.last, f.lastRaw = m, body
	f.mu.Unlock()
	code, resp := f.respond(r.URL.Path, m)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(resp)
}

func okGenerate(string, map[string]any) (int, any) {
	return 200, map[string]any{"responseCode": "2004700", "responseMessage": "Successful", "referenceNo": "DK-REF-1",
		"partnerReferenceNo": "p1", "qrContent": "00020101QR-PAYLOAD"}
}

func createReq() gateway.CreateRequest {
	return gateway.CreateRequest{Reference: "pay-uuid-1", AccountRef: "MID-9", Method: "QRIS", Amount: 10000,
		Currency: "IDR", ExpiresAt: time.Now().Add(15 * time.Minute)}
}

func TestCreateSendsDocumentedRequest(t *testing.T) {
	f, a := newFake(t)
	f.respond = okGenerate
	res, err := a.CreatePayment(context.Background(), createReq())
	if err != nil {
		t.Fatal(err)
	}
	if res.GatewayReference != "DK-REF-1" || res.QRPayload != "00020101QR-PAYLOAD" {
		t.Fatalf("%+v", res)
	}
	m := f.last
	amt := m["amount"].(map[string]any)
	add := m["additionalInfo"].(map[string]any)
	if m["partnerReferenceNo"] != "pay-uuid-1" || amt["value"] != "10000.00" || amt["currency"] != "IDR" ||
		m["merchantId"] != "MID-9" || m["terminalId"] != "TERM01" || add["postalCode"] != "12345" || add["feeType"] != "1" {
		t.Fatalf("body: %v", m)
	}
	// validityPeriod is always explicit (DOKU defaults to 30 days otherwise).
	if v, _ := m["validityPeriod"].(string); !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\+07:00$`).MatchString(v) {
		t.Fatalf("validityPeriod: %v", m["validityPeriod"])
	}
	if strings.Contains(string(f.lastRaw), " ") && strings.Contains(string(f.lastRaw), "\n") {
		t.Fatal("body not minified")
	}
}

func TestTokenIsCachedAndRefreshed(t *testing.T) {
	f, a := newFake(t)
	f.respond = okGenerate
	now := time.Date(2026, 10, 11, 8, 0, 0, 0, time.UTC)
	a.cfg.Now = func() time.Time { return now }
	for i := 0; i < 5; i++ {
		if _, err := a.CreatePayment(context.Background(), createReq()); err != nil {
			t.Fatal(err)
		}
	}
	if f.tokens.Load() != 1 {
		t.Fatalf("token requested %d times, want 1", f.tokens.Load())
	}
	now = now.Add(15 * time.Minute) // past expiry (900s - 60s margin)
	if _, err := a.CreatePayment(context.Background(), createReq()); err != nil {
		t.Fatal(err)
	}
	if f.tokens.Load() != 2 {
		t.Fatalf("token not refreshed: %d", f.tokens.Load())
	}
}

func TestUnauthorizedRefreshesTokenOnceAndRetries(t *testing.T) {
	f, a := newFake(t)
	f.respond = okGenerate
	if _, err := a.CreatePayment(context.Background(), createReq()); err != nil {
		t.Fatal(err)
	}
	f.expire.Store(true)
	if _, err := a.CreatePayment(context.Background(), createReq()); err != nil {
		t.Fatalf("should recover from one 401: %v", err)
	}
	if f.tokens.Load() != 2 {
		t.Fatalf("tokens = %d, want 2", f.tokens.Load())
	}
	// Persistent 401 is a definitive rejection after exactly one retry.
	f.respond = func(string, map[string]any) (int, any) { return 401, map[string]any{"responseCode": "4012500"} }
	before := f.calls.Load()
	_, err := a.CreatePayment(context.Background(), createReq())
	if !gateway.IsRejected(err) || f.calls.Load()-before != 2 {
		t.Fatalf("err=%v calls=%d", err, f.calls.Load()-before)
	}
}

// The core financial rule: anything that MAY have been processed is Ambiguous, not Rejected.
func TestErrorClassification(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		code      string
		ambiguous bool
	}{
		{"bad request", 400, "4004700", false},
		{"unauthorized client", 403, "4034700", false},
		{"rate limited", 429, "4294700", false},
		{"server error", 500, "5004700", true},
		{"gateway timeout code", 504, "5044700", true},
		{"accepted/pending", 202, "2024700", true},
		{"duplicate request id", 409, "4094700", true},
	}
	for _, c := range cases {
		f, a := newFake(t)
		f.respond = func(string, map[string]any) (int, any) {
			return c.status, map[string]any{"responseCode": c.code, "responseMessage": "x"}
		}
		_, err := a.CreatePayment(context.Background(), createReq())
		var ge *gateway.Error
		if !errors.As(err, &ge) {
			t.Fatalf("%s: %v", c.name, err)
		}
		if gateway.IsAmbiguous(err) != c.ambiguous || gateway.IsRejected(err) == c.ambiguous {
			t.Errorf("%s: ambiguous=%v", c.name, gateway.IsAmbiguous(err))
		}
	}
}

func TestTimeoutIsAmbiguous(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(string, map[string]any) (int, any) { time.Sleep(300 * time.Millisecond); return 200, nil }
	a.cfg.HTTP = &http.Client{Timeout: 100 * time.Millisecond}
	_, err := a.CreatePayment(context.Background(), createReq())
	if !gateway.IsAmbiguous(err) {
		t.Fatalf("timeout must be ambiguous: %v", err)
	}
}

func TestIncompleteSuccessIsAmbiguous(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(string, map[string]any) (int, any) { return 200, map[string]any{"responseCode": "2004700"} }
	if _, err := a.CreatePayment(context.Background(), createReq()); !gateway.IsAmbiguous(err) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateValidatesBeforeSending(t *testing.T) {
	f, a := newFake(t)
	f.respond = okGenerate
	bad := []func(*gateway.CreateRequest){
		func(r *gateway.CreateRequest) { r.Amount = 0 },
		func(r *gateway.CreateRequest) { r.Currency = "USD" },
		func(r *gateway.CreateRequest) { r.Reference = strings.Repeat("x", 65) },
		func(r *gateway.CreateRequest) { r.AccountRef = "" },
		func(r *gateway.CreateRequest) { r.ExpiresAt = time.Time{} },
	}
	for i, m := range bad {
		r := createReq()
		m(&r)
		if _, err := a.CreatePayment(context.Background(), r); !gateway.IsRejected(err) {
			t.Errorf("case %d: %v", i, err)
		}
	}
	r := createReq()
	r.Method = "VA"
	if _, err := a.CreatePayment(context.Background(), r); !errors.Is(err, gateway.ErrNotSupported) {
		t.Errorf("method: %v", err)
	}
	if f.calls.Load() != 0 || f.tokens.Load() != 0 {
		t.Fatal("invalid requests reached the gateway")
	}
}

func TestQueryRequestAndResponse(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(path string, _ map[string]any) (int, any) {
		if path != pathQuery {
			t.Errorf("path %s", path)
		}
		return 200, map[string]any{"responseCode": "2004700", "latestTransactionStatus": "00",
			"amount": map[string]string{"value": "10000.00", "currency": "IDR"}, "paidTime": "2026-10-11T08:00:00+07:00"}
	}
	res, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "p1", GatewayReference: "DK-REF-1", AccountRef: "MID-9"})
	if err != nil {
		t.Fatal(err)
	}
	if res.RawStatus != "00" || res.Amount != 10000 || res.Currency != "IDR" || res.PaidAt == nil {
		t.Fatalf("%+v", res)
	}
	m := f.last
	if m["originalReferenceNo"] != "DK-REF-1" || m["originalPartnerReferenceNo"] != "p1" || m["serviceCode"] != "47" || m["merchantId"] != "MID-9" {
		t.Fatalf("query body: %v", m)
	}
	// An amount in an unexpected shape is reported as unusable (-1), never guessed.
	f.respond = func(string, map[string]any) (int, any) {
		return 200, map[string]any{"responseCode": "2004700", "latestTransactionStatus": "00", "amount": map[string]string{"value": "10000.50", "currency": "IDR"}}
	}
	res, _ = a.GetPayment(context.Background(), gateway.QueryRequest{Reference: "p1", GatewayReference: "r", AccountRef: "m"})
	if res.Amount != -1 {
		t.Fatalf("amount = %d", res.Amount)
	}
}

func TestCancelRequest(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(path string, _ map[string]any) (int, any) {
		if path != pathExpire {
			t.Errorf("path %s", path)
		}
		return 200, map[string]any{"responseCode": "2004700"}
	}
	if err := a.CancelPayment(context.Background(), gateway.CancelRequest{Reference: "p1", GatewayReference: "DK-REF-1", AccountRef: "MID-9"}); err != nil {
		t.Fatal(err)
	}
	m := f.last
	if m["partnerReferenceNo"] != "p1" || m["referenceNo"] != "DK-REF-1" || m["merchantId"] != "MID-9" {
		t.Fatalf("%v", m)
	}
}

func TestStatusMapIsOffUntilConfirmed(t *testing.T) {
	_, a := newFake(t)
	for _, raw := range []string{"00", "01", "05", "06", "", "xx"} {
		if _, ok := a.MapStatus(raw); ok {
			t.Fatalf("unconfirmed mapping returned a status for %q", raw)
		}
	}
	a.cfg.StatusMapConfirmed = true
	want := map[string]gateway.Status{"00": gateway.StatusSucceeded, "01": gateway.StatusPending, "02": gateway.StatusPending,
		"03": gateway.StatusPending, "05": gateway.StatusCancelled, "06": gateway.StatusFailed}
	for raw, w := range want {
		if got, ok := a.MapStatus(raw); !ok || got != w {
			t.Errorf("%s: %v %v", raw, got, ok)
		}
	}
	for _, raw := range []string{"04", "07", "", "99"} { // refunded / not-found / unknown stay unmapped
		if _, ok := a.MapStatus(raw); ok {
			t.Errorf("%q must stay unmapped", raw)
		}
	}
}

func TestNewValidatesConfig(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	ok := Config{ClientID: "c", ClientSecret: "s", PrivateKey: key, TerminalID: "TERM01", PostalCode: "1"}
	if _, err := New(ok); err != nil {
		t.Fatal(err)
	}
	for _, mut := range []func(*Config){
		func(c *Config) { c.ClientID = "" }, func(c *Config) { c.ClientSecret = "" }, func(c *Config) { c.PrivateKey = nil },
		func(c *Config) { c.TerminalID = "x" }, func(c *Config) { c.TerminalID = "bad id!" }, func(c *Config) { c.PostalCode = "" },
	} {
		c := ok
		mut(&c)
		if _, err := New(c); err == nil {
			t.Error("invalid config accepted")
		}
	}
	if _, err := ParsePrivateKey([]byte("not pem")); err == nil {
		t.Error("garbage key accepted")
	}
}

func TestErrorsNeverCarrySecrets(t *testing.T) {
	f, a := newFake(t)
	f.respond = func(string, map[string]any) (int, any) {
		return 400, map[string]any{"responseCode": "4004700", "responseMessage": "bad\nvalue " + f.secret}
	}
	_, err := a.CreatePayment(context.Background(), createReq())
	if err == nil || strings.Contains(err.Error(), "\n") {
		t.Fatalf("%v", err)
	}
	for _, s := range []string{clientID + "|", "tok-"} {
		if strings.Contains(err.Error(), s) {
			t.Fatalf("error leaks %q", s)
		}
	}
}
