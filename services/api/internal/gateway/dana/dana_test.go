package dana

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const payID = "0192f3a4-7b1c-7d2e-9a3b-4c5d6e7f8091"

var now = time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC)

// keys: ours (signs requests) and "DANA's" (signs webhooks), generated once.
var merchantKey, danaKey = mustKey(), mustKey()

func mustKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func privPEM(k *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(k)}))
}

func mustPKCS8(k *rsa.PrivateKey) []byte {
	b, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		panic(err)
	}
	return b
}

func pubPEM(k *rsa.PublicKey) string {
	b, _ := x509.MarshalPKIXPublicKey(k)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b}))
}

func newAdapter(t *testing.T, h http.HandlerFunc) *Adapter {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a, err := New(Config{BaseURL: srv.URL, MerchantID: "M1", ClientID: "C1", StoreID: "S1", ChannelID: "95221",
		PrivateKeyPEM: privPEM(merchantKey), PublicKeyPEM: pubPEM(&danaKey.PublicKey), Now: func() time.Time { return now }})
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
	return a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: payID, Method: "QRIS", Amount: 10000,
		Currency: "IDR", ExpiresAt: now.Add(30 * time.Minute)})
}

func TestShortRefRoundTrip(t *testing.T) {
	s, ok := shortRef(payID)
	if !ok || len(s) != 25 {
		t.Fatalf("shortRef = %q %v", s, ok)
	}
	if back, ok := LongRef(s); !ok || back != payID {
		t.Fatalf("LongRef = %q %v", back, ok)
	}
	// extremes still fit in 25 characters
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "ffffffff-ffff-ffff-ffff-ffffffffffff"} {
		if s, ok := shortRef(id); !ok || len(s) != 25 {
			t.Errorf("%s -> %q %v", id, s, ok)
		}
	}
	for _, bad := range []string{"", "not-a-uuid", strings.ToUpper(payID), payID + "0"} {
		if _, ok := shortRef(bad); ok {
			t.Errorf("shortRef(%q) accepted", bad)
		}
	}
}

func TestMoney(t *testing.T) {
	if fmtMoney(10000) != "10000.00" {
		t.Error(fmtMoney(10000))
	}
	for s, want := range map[string]int64{"10000.00": 10000, "10000": 10000, "10000.0": 10000} {
		if v, ok := parseMoney(s); !ok || v != want {
			t.Errorf("parseMoney(%q) = %d %v", s, v, ok)
		}
	}
	for _, s := range []string{"10000.50", "10000.005", "-1.00", "abc", ""} {
		if _, ok := parseMoney(s); ok {
			t.Errorf("parseMoney(%q) accepted", s)
		}
	}
}

// The request must be signed exactly as DANA documents: DANA verifies it with our public key.
func TestCreateSignsAndSendsDocumentedRequest(t *testing.T) {
	a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || r.URL.Path != pathGenerate {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		for h, want := range map[string]string{"X-PARTNER-ID": "C1", "CHANNEL-ID": "95221", "Content-Type": "application/json"} {
			if r.Header.Get(h) != want {
				t.Errorf("%s = %q", h, r.Header.Get(h))
			}
		}
		if e := r.Header.Get("X-EXTERNAL-ID"); len(e) == 0 || len(e) > 36 {
			t.Errorf("X-EXTERNAL-ID = %q", e)
		}
		ts := r.Header.Get("X-TIMESTAMP")
		if ts != "2026-11-20T14:00:00+07:00" {
			t.Errorf("X-TIMESTAMP = %q", ts)
		}
		sig, _ := base64.StdEncoding.DecodeString(r.Header.Get("X-SIGNATURE"))
		h := sha256.Sum256([]byte(stringToSign("POST", pathGenerate, body, ts)))
		if rsa.VerifyPKCS1v15(&merchantKey.PublicKey, crypto.SHA256, h[:], sig) != nil {
			t.Error("X-SIGNATURE does not verify with the merchant public key")
		}
		if bytes.ContainsAny(body, " \n") && !strings.Contains(string(body), "IPG") {
			t.Error("body not minified")
		}
		var got map[string]any
		_ = json.Unmarshal(body, &got)
		amt := got["amount"].(map[string]any)
		ref, _ := shortRef(payID)
		if got["merchantId"] != "M1" || got["storeId"] != "S1" || got["partnerReferenceNo"] != ref ||
			amt["value"] != "10000.00" || amt["currency"] != "IDR" || got["validityPeriod"] != "2026-11-20T14:30:00+07:00" {
			t.Errorf("body = %s", body)
		}
		reply(w, 200, `{"responseCode":"2004700","responseMessage":"Successful","referenceNo":"D-1","qrContent":"000201"}`)
	})
	res, err := create(a)
	if err != nil || res.QRPayload != "000201" || res.GatewayReference != "D-1" || res.RawStatus != "01" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestCreateOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string // ok | rejected | ambiguous
	}{
		{"invalid mandatory field", 400, `{"responseCode":"4004702"}`, "rejected"},
		{"unauthorized", 401, `{"responseCode":"4014700"}`, "rejected"},
		{"amount limit", 403, `{"responseCode":"4034702"}`, "rejected"},
		{"inconsistent request", 404, `{"responseCode":"4044718"}`, "rejected"},
		{"too many requests", 429, `{"responseCode":"4294700"}`, "rejected"},
		{"general error non retry-able", 500, `{"responseCode":"5004700"}`, "rejected"},
		{"internal server error", 500, `{"responseCode":"5004701"}`, "ambiguous"},
		{"5xx without body", 502, `{}`, "ambiguous"},
		{"202 in progress", 202, `{"responseCode":"2024700"}`, "ambiguous"},
		{"200 without QR", 200, `{"responseCode":"2004700"}`, "ambiguous"},
		{"200 with unknown code", 200, `{"responseCode":"2009999","qrContent":"000201"}`, "ambiguous"},
		{"html 4xx", 404, `<html>`, "rejected"},
	}
	for _, c := range cases {
		a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, c.status, c.body) })
		_, err := create(a)
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
	a.cfg.BaseURL = "http://127.0.0.1:1"
	if _, err := create(a); !gateway.IsAmbiguous(err) {
		t.Fatalf("err = %v, want ambiguous", err)
	}
}

func TestCreateValidation(t *testing.T) {
	a := newAdapter(t, func(http.ResponseWriter, *http.Request) { t.Error("must not call gateway") })
	bad := []gateway.CreateRequest{
		{Reference: "not-a-uuid", Method: "QRIS", Amount: 1, Currency: "IDR", ExpiresAt: now.Add(time.Hour)},
		{Reference: payID, Method: "QRIS", Amount: 0, Currency: "IDR", ExpiresAt: now.Add(time.Hour)},
		{Reference: payID, Method: "QRIS", Amount: 1, Currency: "USD", ExpiresAt: now.Add(time.Hour)},
		{Reference: payID, Method: "QRIS", Amount: 1, Currency: "IDR", ExpiresAt: now.Add(-time.Minute)},
		{Reference: payID, Method: "QRIS", Amount: 1, Currency: "IDR"},
	}
	for i, r := range bad {
		if _, err := a.CreatePayment(context.Background(), r); !gateway.IsRejected(err) {
			t.Errorf("case %d: err = %v", i, err)
		}
	}
	if _, err := a.CreatePayment(context.Background(), gateway.CreateRequest{Reference: payID, Method: "VA"}); err != gateway.ErrNotSupported {
		t.Errorf("method VA: %v", err)
	}
}

func TestGetPayment(t *testing.T) {
	cases := []struct {
		name, body, wantRaw string
		status              int
		wantAmt             int64
		wantErr             string
	}{
		{"paid", `{"responseCode":"2005500","latestTransactionStatus":"00","transAmount":{"value":"10000.00","currency":"IDR"},"paidTime":"2026-11-20T14:05:00+07:00"}`, "00", 200, 10000, ""},
		{"initiated", `{"responseCode":"2005500","latestTransactionStatus":"01"}`, "01", 200, -1, ""},
		{"closed", `{"responseCode":"2005500","latestTransactionStatus":"05"}`, "05", 200, -1, ""},
		{"not found code", `{"responseCode":"4045501"}`, "07", 404, -1, ""},
		{"success without status", `{"responseCode":"2005500"}`, "", 200, 0, "ambiguous"},
		{"server error", `{"responseCode":"5005501"}`, "", 500, 0, "ambiguous"},
		{"bad signature", `{"responseCode":"4015500"}`, "", 401, 0, "rejected"},
	}
	for _, c := range cases {
		a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			ref, _ := shortRef(payID)
			if r.URL.Path != pathQuery || !strings.Contains(string(body), `"originalPartnerReferenceNo":"`+ref+`"`) || !strings.Contains(string(body), `"serviceCode":"47"`) {
				t.Errorf("%s: %s %s", c.name, r.URL.Path, body)
			}
			reply(w, c.status, c.body)
		})
		res, err := a.GetPayment(context.Background(), gateway.QueryRequest{Reference: payID})
		switch c.wantErr {
		case "":
			if err != nil || res.RawStatus != c.wantRaw || res.Amount != c.wantAmt {
				t.Errorf("%s: res=%+v err=%v", c.name, res, err)
			}
		case "ambiguous":
			if !gateway.IsAmbiguous(err) {
				t.Errorf("%s: err=%v", c.name, err)
			}
		case "rejected":
			if !gateway.IsRejected(err) {
				t.Errorf("%s: err=%v", c.name, err)
			}
		}
		if c.name == "paid" && (res.PaidAt == nil || res.PaidAt.UTC().Hour() != 7 || res.PaidAt.UTC().Minute() != 5) {
			t.Errorf("PaidAt = %v", res.PaidAt)
		}
	}
}

func TestMapStatus(t *testing.T) {
	a := newAdapter(t, nil)
	want := map[string]gateway.Status{"00": gateway.StatusSucceeded, "01": gateway.StatusPending, "02": gateway.StatusPending,
		"05": gateway.StatusExpired, "07": gateway.StatusFailed}
	for raw, st := range want {
		if got, ok := a.MapStatus(raw); !ok || got != st {
			t.Errorf("%s -> %v %v", raw, got, ok)
		}
	}
	if _, ok := a.MapStatus("99"); ok {
		t.Error("unknown status must stay unmapped")
	}
}

func TestTestConnection(t *testing.T) {
	a := newAdapter(t, func(w http.ResponseWriter, _ *http.Request) { reply(w, 404, `{"responseCode":"4045501"}`) })
	if st := a.TestConnection(context.Background(), "", false); !st[0].OK {
		t.Errorf("valid credentials: %+v", st)
	}
	a = newAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		reply(w, 401, `{"responseCode":"4015500","responseMessage":"Unauthorized"}`)
	})
	if st := a.TestConnection(context.Background(), "", false); st[0].OK || st[0].Code != "4015500" {
		t.Errorf("bad credentials: %+v", st)
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	ok := Config{BaseURL: SandboxURL, MerchantID: "M", ClientID: "C", StoreID: "S", ChannelID: "1", PrivateKeyPEM: privPEM(merchantKey)}
	if _, err := New(ok); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Config){
		"no store":        func(c *Config) { c.StoreID = "" },
		"long channel":    func(c *Config) { c.ChannelID = "123456" },
		"bad url":         func(c *Config) { c.BaseURL = "api" },
		"bad private key": func(c *Config) { c.PrivateKeyPEM = "nope" },
		"bad public key":  func(c *Config) { c.PublicKeyPEM = "nope" },
	} {
		c := ok
		mut(&c)
		if _, err := New(c); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// ---- webhook ----

func notifyBody(status, amount string) []byte {
	ref, _ := shortRef(payID)
	return []byte(`{"originalPartnerReferenceNo":"` + ref + `","originalReferenceNo":"D-1","merchantId":"M1",` +
		`"amount":{"value":"` + amount + `","currency":"IDR"},"latestTransactionStatus":"` + status + `"}`)
}

// signed builds headers the way DANA does: its private key over POST:path:hash:timestamp.
func signed(path string, body []byte) http.Header {
	ts := "2026-11-20T14:05:00+07:00"
	var min bytes.Buffer
	_ = json.Compact(&min, body)
	h := sha256.Sum256([]byte(stringToSign("POST", path, min.Bytes(), ts)))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, danaKey, crypto.SHA256, h[:])
	return http.Header{"X-Signature": {base64.StdEncoding.EncodeToString(sig)}, "X-Timestamp": {ts}}
}

func TestVerifyWebhook(t *testing.T) {
	a := newAdapter(t, nil)
	const path = "/webhooks/dana"
	body := notifyBody("00", "10000.00")
	ev, err := a.VerifyWebhook(signed(path, body), path, body)
	if err != nil {
		t.Fatal(err)
	}
	if ev.PaymentRef != payID || ev.Status != gateway.StatusSucceeded || !ev.StatusOK || ev.Amount != 10000 || ev.Currency != "IDR" {
		t.Errorf("event = %+v", ev)
	}
	// Duplicate delivery -> same dedup key; a different status -> a different one.
	ev2, _ := a.VerifyWebhook(signed(path, body), path, body)
	exp := notifyBody("05", "10000.00")
	ev3, _ := a.VerifyWebhook(signed(path, exp), path, exp)
	if ev.DedupKey != ev2.DedupKey || ev.DedupKey == ev3.DedupKey || ev3.Status != gateway.StatusExpired {
		t.Errorf("dedup keys: %q %q %q", ev.DedupKey, ev2.DedupKey, ev3.DedupKey)
	}
	// DANA may sign its documented path instead of the registered URL path: both are accepted.
	if _, err := a.VerifyWebhook(signed(pathNotify, body), path, body); err != nil {
		t.Errorf("documented notify path: %v", err)
	}
	// Whitespace in the body does not break the signature (DANA signs the minified form).
	pretty := []byte(strings.ReplaceAll(string(body), ",", ",\n  "))
	if _, err := a.VerifyWebhook(signed(path, body), path, pretty); err != nil {
		t.Errorf("pretty body: %v", err)
	}
}

func TestVerifyWebhookRejects(t *testing.T) {
	a := newAdapter(t, nil)
	const path = "/webhooks/dana"
	body := notifyBody("00", "10000.00")
	tampered := notifyBody("00", "99999.00")
	other := signed(path, body)
	other.Set("X-Signature", base64.StdEncoding.EncodeToString([]byte("garbage")))
	cases := map[string]struct {
		h    http.Header
		path string
		body []byte
	}{
		"missing headers": {http.Header{}, path, body},
		"tampered body":   {signed(path, body), path, tampered},
		"wrong path":      {signed(path, body), "/other", body},
		"garbage sig":     {other, path, body},
		"signed by merchant, not DANA": {func() http.Header {
			h := signed(path, body)
			var min bytes.Buffer
			_ = json.Compact(&min, body)
			d := sha256.Sum256([]byte(stringToSign("POST", path, min.Bytes(), h.Get("X-Timestamp"))))
			s, _ := rsa.SignPKCS1v15(rand.Reader, merchantKey, crypto.SHA256, d[:])
			h.Set("X-Signature", base64.StdEncoding.EncodeToString(s))
			return h
		}(), path, body},
	}
	for name, c := range cases {
		if _, err := a.VerifyWebhook(c.h, c.path, c.body); err == nil || !isInvalidSig(err) {
			t.Errorf("%s: err = %v, want invalid signature", name, err)
		}
	}
	// Authentic but unusable payloads are not signature errors.
	for name, b := range map[string][]byte{
		"fraction":       notifyBody("00", "10000.50"),
		"other merchant": []byte(strings.Replace(string(body), `"M1"`, `"M2"`, 1)),
		"unknown ref":    []byte(strings.Replace(string(body), "originalPartnerReferenceNo\":\"", "originalPartnerReferenceNo\":\"x", 1)),
	} {
		if _, err := a.VerifyWebhook(signed(path, b), path, b); err == nil || isInvalidSig(err) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// No public key configured: nothing can be authenticated.
	na, _ := New(Config{BaseURL: SandboxURL, MerchantID: "M1", ClientID: "C1", StoreID: "S1", ChannelID: "1", PrivateKeyPEM: privPEM(merchantKey)})
	if _, err := na.VerifyWebhook(signed(path, body), path, body); !isInvalidSig(err) {
		t.Errorf("no public key: %v", err)
	}
}

func isInvalidSig(err error) bool {
	var se *gateway.SignatureError
	return err != nil && (err == gateway.ErrInvalidSignature || (asSig(err, &se)))
}

func asSig(err error, se **gateway.SignatureError) bool {
	s, ok := err.(*gateway.SignatureError)
	*se = s
	return ok
}

// ---- from DANA's official Go SDK (dana-id/dana-go) ----

func TestChannelIDDefaultsToSDKValue(t *testing.T) {
	var got string
	a := newAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("CHANNEL-ID")
		reply(w, 200, `{"responseCode":"2004700","qrContent":"000201"}`)
	})
	a.cfg.ChannelID = "" // not through New: prove the default is applied by New itself below
	c, err := New(Config{BaseURL: a.cfg.BaseURL, MerchantID: "M1", ClientID: "C1", StoreID: "S1", PrivateKeyPEM: privPEM(merchantKey), Now: a.cfg.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := create(c); err != nil || got != "95221" {
		t.Fatalf("CHANNEL-ID = %q err=%v", got, err)
	}
}

// DANA's dashboard copies keys as bare base64 (no BEGIN/END lines); both key fields must accept that.
func TestKeysAcceptBareBase64(t *testing.T) {
	bare := func(pemText string) string {
		b, _ := pem.Decode([]byte(pemText))
		return base64.StdEncoding.EncodeToString(b.Bytes)
	}
	a, err := New(Config{BaseURL: SandboxURL, MerchantID: "M1", ClientID: "C1", StoreID: "S1",
		PrivateKeyPEM: bare(privPEM(merchantKey)), PublicKeyPEM: bare(pubPEM(&danaKey.PublicKey))})
	if err != nil || a.priv == nil || a.pub == nil {
		t.Fatalf("bare base64 keys rejected: %v", err)
	}
	// PKCS#1 private key, bare as well.
	if _, err := New(Config{BaseURL: SandboxURL, MerchantID: "M1", ClientID: "C1", StoreID: "S1",
		PrivateKeyPEM: base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(merchantKey))}); err != nil {
		t.Errorf("bare PKCS#1: %v", err)
	}
}

// DANA's SDK also accepts a body signed over the exact bytes received (not only the minified form).
func TestVerifyWebhookAcceptsSignatureOverRawBytes(t *testing.T) {
	a := newAdapter(t, nil)
	const path = "/webhooks/sandbox/dana"
	raw := []byte("{\"originalPartnerReferenceNo\": \"" + func() string { r, _ := shortRef(payID); return r }() + "\",\n \"originalReferenceNo\":\"D-1\",\"merchantId\":\"M1\"," +
		"\"amount\":{\"value\":\"10000.00\",\"currency\":\"IDR\"},\"latestTransactionStatus\":\"00\"}")
	ts := "2026-11-20T14:05:00+07:00"
	h := sha256.Sum256([]byte(stringToSign("POST", path, raw, ts)))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, danaKey, crypto.SHA256, h[:])
	hdr := http.Header{"X-Signature": {base64.StdEncoding.EncodeToString(sig)}, "X-Timestamp": {ts}}
	if ev, err := a.VerifyWebhook(hdr, path, raw); err != nil || ev.PaymentRef != payID {
		t.Fatalf("raw-bytes signature: ev=%+v err=%v", ev, err)
	}
}

// The debug header is a sandbox-only convenience: it must never be sent to production.
func TestDebugHeaderOnlyInSandbox(t *testing.T) {
	for host, want := range map[string]string{SandboxURL: "true", ProductionURL: ""} {
		var got string
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			got = r.Header.Get("X-Debug-Mode")
			return &http.Response{StatusCode: 400, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(
				`{"responseCode":"4004702","responseMessage":"Invalid Mandatory Field","additionalInfo":{"debugMessage":"storeId not registered"}}`))}, nil
		})
		a, _ := New(Config{BaseURL: host, MerchantID: "M1", ClientID: "C1", StoreID: "S1", PrivateKeyPEM: privPEM(merchantKey),
			HTTP: &http.Client{Transport: rt}, Now: func() time.Time { return now }})
		_, err := create(a)
		if got != want {
			t.Errorf("%s: X-Debug-Mode = %q, want %q", host, got, want)
		}
		if ge, ok := err.(*gateway.Error); !ok || !strings.Contains(ge.Message, "storeId not registered") {
			t.Errorf("%s: debug reason not surfaced: %v", host, err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
