package qrisgate

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/qrisgate"
)

const (
	merchantA = "11111111-1111-1111-1111-111111111111"
	merchantB = "22222222-2222-2222-2222-222222222222"
	payID     = "33333333-3333-3333-3333-333333333333"
	secretA   = "qls_secretA-0123456789"
)

// fakePort stands in for the domain service.
type fakePort struct {
	secrets    map[string]string
	claimErr   error
	expired    bool
	found      bool
	matchPID   string
	matchErr   error
	account    *qrisgate.Account
	accountErr error
	got        struct {
		pkg, title, text string
		amount           int64
		delivery         string
		posted           time.Time
	}
}

func (f *fakePort) Claim(_ context.Context, merchantID, paymentID string, base int64, _ time.Time) (int64, string, error) {
	if f.claimErr != nil {
		return 0, "", f.claimErr
	}
	return base + 7, "QR:" + strconv.FormatInt(base+7, 10), nil
}
func (f *fakePort) ClaimExpired(context.Context, string) (bool, bool, error) {
	return f.found, f.expired, nil
}
func (f *fakePort) ListenerSecret(_ context.Context, m string) (string, error) {
	if s, ok := f.secrets[m]; ok {
		return s, nil
	}
	return "", qrisgate.ErrNoAccount
}
func (f *fakePort) Account(_ context.Context, m string) (qrisgate.Account, error) {
	if f.accountErr != nil {
		return qrisgate.Account{}, f.accountErr
	}
	if f.account != nil {
		return *f.account, nil
	}
	return qrisgate.Account{Packages: []string{}}, nil
}
func (f *fakePort) MatchNotification(_ context.Context, m, delivery, pkg, title, text string, amount int64, posted time.Time) (string, int64, bool, error) {
	f.got.pkg, f.got.title, f.got.text, f.got.amount, f.got.delivery, f.got.posted = pkg, title, text, amount, delivery, posted
	if f.matchErr != nil {
		return "", 0, false, f.matchErr
	}
	return f.matchPID, 25_000, f.matchPID != "", nil
}

var now0 = time.Unix(1_700_000_000, 0)

func newAdapter(t *testing.T, f *fakePort) *Adapter {
	t.Helper()
	if f.secrets == nil {
		f.secrets = map[string]string{merchantA: secretA}
	}
	a, err := New(Config{Port: f, Now: func() time.Time { return now0 }})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func createReq() gateway.CreateRequest {
	return gateway.CreateRequest{Reference: payID, AccountRef: merchantA, Method: "QRIS", Amount: 25_000, Currency: "IDR", ExpiresAt: now0.Add(15 * time.Minute)}
}

func TestCreateReturnsTheDynamicQRAndClassifiesFailures(t *testing.T) {
	a := newAdapter(t, &fakePort{})
	res, err := a.CreatePayment(context.Background(), createReq())
	if err != nil || res.QRPayload != "QR:25007" || res.RawStatus != "PENDING" || res.GatewayReference != "own-"+payID {
		t.Fatalf("%+v %v", res, err)
	}
	if st, ok := a.MapStatus(res.RawStatus); !ok || st != gateway.StatusPending {
		t.Fatal("PENDING must map to pending")
	}
	for name, c := range map[string]struct {
		err  error
		want gateway.Kind
	}{
		"no QRIS saved":   {qrisgate.ErrNoQRIS, gateway.Rejected},
		"no free code":    {qrisgate.ErrNoFreeCode, gateway.Rejected},
		"database broken": {errors.New("db down"), gateway.Ambiguous},
	} {
		a := newAdapter(t, &fakePort{claimErr: c.err})
		_, err := a.CreatePayment(context.Background(), createReq())
		var ge *gateway.Error
		if !errors.As(err, &ge) || ge.Kind != c.want {
			t.Errorf("%s: %v; want kind %v", name, err, c.want)
		}
	}
	for name, mod := range map[string]func(*gateway.CreateRequest){
		"other method":     func(r *gateway.CreateRequest) { r.Method = "VA" },
		"bad merchant ref": func(r *gateway.CreateRequest) { r.AccountRef = "x" },
		"bad payment ref":  func(r *gateway.CreateRequest) { r.Reference = "x" },
		"zero amount":      func(r *gateway.CreateRequest) { r.Amount = 0 },
		"no expiry":        func(r *gateway.CreateRequest) { r.ExpiresAt = time.Time{} },
	} {
		r := createReq()
		mod(&r)
		if _, err := a.CreatePayment(context.Background(), r); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestInquiryOnlyEverReportsPendingOrExpired(t *testing.T) {
	for name, c := range map[string]struct {
		f    fakePort
		want string
	}{"open": {fakePort{found: true}, "PENDING"}, "ran out": {fakePort{found: true, expired: true}, "EXPIRED"}} {
		q, err := newAdapter(t, &c.f).GetPayment(context.Background(), gateway.QueryRequest{Reference: payID, AccountRef: merchantA})
		if err != nil || q.RawStatus != c.want || q.Amount != -1 {
			t.Errorf("%s: %+v %v", name, q, err)
		}
	}
	if _, err := newAdapter(t, &fakePort{}).GetPayment(context.Background(), gateway.QueryRequest{Reference: payID, AccountRef: merchantA}); !gateway.IsAmbiguous(err) {
		t.Errorf("no claim: %v; want ambiguous", err)
	}
}

func hook(secret string, ts int64, body, delivery string) http.Header {
	h := http.Header{}
	h.Set("X-Listener-Timestamp", strconv.FormatInt(ts, 10))
	h.Set("X-Listener-Signature", "sha256="+Signature(secret, ts, []byte(body)))
	h.Set("X-Listener-Delivery-Id", delivery)
	return h
}

const notif = `{"event":"notification.posted","delivery_id":"d-1","package":"id.dana","title":"Dana masuk","text":"Rp 25.007 dari BUDI","amount":25007,"posted_at":"2026-11-20T10:00:00Z"}`

func TestWebhookMatchesSignedNotificationsOnly(t *testing.T) {
	f := &fakePort{matchPID: payID}
	a := newAdapter(t, f)
	path := "/webhooks/qrisgate/" + merchantA
	ts := now0.Unix()

	ev, err := a.VerifyWebhook(hook(secretA, ts, notif, "d-1"), path, []byte(notif))
	if err != nil || ev.PaymentRef != payID || ev.Status != gateway.StatusSucceeded || !ev.StatusOK || ev.Amount != 25_000 || ev.MerchantID != merchantA || ev.DedupKey != "qrisgate:listener:d-1" {
		t.Fatalf("matched: %+v %v", ev, err)
	}
	if f.got.pkg != "id.dana" || f.got.amount != 25007 || f.got.delivery != "d-1" || !strings.Contains(f.got.text, "BUDI") || f.got.posted.Year() != 2026 {
		t.Fatalf("what the domain was asked: %+v", f.got)
	}
	// Unmatched, and a test event, are acknowledged without a payment.
	f.matchPID = ""
	if ev, err := a.VerifyWebhook(hook(secretA, ts, notif, "d-2"), path, []byte(notif)); err != nil || ev.PaymentRef != "" || ev.StatusOK {
		t.Fatalf("unmatched: %+v %v", ev, err)
	}
	test := `{"event":"listener.test","delivery_id":"t","package":"x.y","title":"Tes","text":"Rp 10.000"}`
	if ev, err := a.VerifyWebhook(hook(secretA, ts, test, "t"), path, []byte(test)); err != nil || ev.PaymentRef != "" || ev.RawStatus != "TEST" {
		t.Fatalf("test event: %+v %v", ev, err)
	}
	// A transient database failure is NOT an invalid signature (the listener must retry).
	f.matchErr = errors.New("db down")
	if _, err := a.VerifyWebhook(hook(secretA, ts, notif, "d-3"), path, []byte(notif)); err == nil || errors.Is(err, gateway.ErrInvalidSignature) {
		t.Fatalf("transient failure: %v", err)
	}
	f.matchErr = nil

	reason := func(h http.Header, p string, b string) string {
		_, err := a.VerifyWebhook(h, p, []byte(b))
		var se *gateway.SignatureError
		if !errors.As(err, &se) || !errors.Is(err, gateway.ErrInvalidSignature) {
			t.Fatalf("not a signature error: %v", err)
		}
		return se.Reason
	}
	noTS := hook(secretA, ts, notif, "d")
	noTS.Del("X-Listener-Timestamp")
	for want, got := range map[string]string{
		"no_merchant_id_in_path":          reason(http.Header{}, "/webhooks/qrisgate", notif),
		"missing_signature_header":        reason(http.Header{}, path, notif),
		"missing_or_bad_timestamp_header": reason(noTS, path, notif),
		"timestamp_skew:400s":             reason(hook(secretA, ts-400, notif, "d"), path, notif),
		"no_listener_secret_for_merchant": reason(hook(secretA, ts, notif, "d"), "/webhooks/qrisgate/"+merchantB, notif),
		"signature_mismatch":              reason(hook("qls_wrong-wrong-wrong-0", ts, notif, "d"), path, notif),
	} {
		if got != want {
			t.Errorf("reason %q; want %q", got, want)
		}
		if strings.Contains(got, "secret") && !strings.Contains(got, "no_listener_secret") {
			t.Errorf("reason leaks: %q", got)
		}
	}
	if got := reason(hook(secretA, ts, notif, "d"), path, notif+" "); got != "signature_mismatch" {
		t.Errorf("tampered body: %q", got)
	}
}

func TestTestWebhookNeverLeaksTheSecret(t *testing.T) {
	a := newAdapter(t, &fakePort{})
	steps := a.TestWebhook(context.Background(), merchantA)
	if len(steps) != 2 || !steps[0].OK || !steps[1].OK {
		t.Fatalf("configured: %+v", steps)
	}
	for _, st := range steps {
		if strings.Contains(st.Message+st.Code, "secretA") {
			t.Fatalf("leak: %+v", st)
		}
	}
	if steps = a.TestWebhook(context.Background(), merchantB); len(steps) != 1 || steps[0].OK || steps[0].Code != "no_secret" {
		t.Fatalf("no secret: %+v", steps)
	}
	if steps = a.TestWebhook(context.Background(), "nope"); steps[0].OK {
		t.Fatalf("bad id: %+v", steps)
	}
}

func TestTestConnection(t *testing.T) {
	ctx := context.Background()
	ready := &qrisgate.Account{HasQRIS: true, HasSecret: true, Packages: []string{"id.dana"}, Ready: true, ActivationStatus: "active"}
	ok := func(steps []gateway.ConnectionStep) bool {
		for _, s := range steps {
			if !s.OK {
				return false
			}
		}
		return len(steps) > 0
	}
	// Tanpa deep: tidak ada kredensial eksternal, tidak menyentuh data merchant.
	if steps := newAdapter(t, &fakePort{}).TestConnection(ctx, "", false); !ok(steps) {
		t.Fatalf("shallow: %+v", steps)
	}
	if steps := newAdapter(t, &fakePort{account: ready}).TestConnection(ctx, merchantA, true); !ok(steps) {
		t.Fatalf("deep ready: %+v", steps)
	}
	for name, acc := range map[string]*qrisgate.Account{
		"no_qris":     {HasSecret: true, Packages: []string{"id.dana"}, ActivationStatus: "active"},
		"no_packages": {HasQRIS: true, HasSecret: true, Packages: []string{}, ActivationStatus: "active"},
		"not_active":  {HasQRIS: true, HasSecret: true, Packages: []string{"id.dana"}, ActivationStatus: "pending_confirmation"},
	} {
		steps := newAdapter(t, &fakePort{account: acc}).TestConnection(ctx, merchantA, true)
		bad := ""
		for _, s := range steps {
			if !s.OK {
				bad = s.Code
			}
		}
		if bad != name {
			t.Errorf("%s: failing code = %q (%+v)", name, bad, steps)
		}
	}
	if steps := newAdapter(t, &fakePort{}).TestConnection(ctx, merchantB, true); ok(steps) { // tanpa rahasia listener
		t.Fatalf("no secret must fail: %+v", steps)
	}
	if steps := newAdapter(t, &fakePort{accountErr: errors.New("db")}).TestConnection(ctx, merchantA, true); ok(steps) || strings.Contains(steps[0].Message, "db") {
		t.Fatalf("db error: %+v", steps)
	}
}
