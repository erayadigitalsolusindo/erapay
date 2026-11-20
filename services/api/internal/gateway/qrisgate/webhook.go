package qrisgate

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// MaxSkew is how far the notification timestamp may differ from now.
const MaxSkew = 300 * time.Second

// Signature is hex(HMAC-SHA256(secret, timestamp + "." + rawBody)), the scheme apps/listener-android signs with.
func Signature(secret string, timestamp int64, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

type notification struct {
	Event      string `json:"event"`
	DeliveryID string `json:"delivery_id"`
	Package    string `json:"package"`
	Title      string `json:"title"`
	Text       string `json:"text"`
	Amount     *int64 `json:"amount"`
	PostedAt   string `json:"posted_at"`
}

// merchantFromPath reads the merchant id out of /webhooks/qrisgate/<merchant_id>: the listener is configured with
// that URL, and the secret that verifies it is that merchant's own.
func merchantFromPath(path string) (string, bool) {
	i := strings.Index(path, "/qrisgate/")
	if i < 0 {
		return "", false
	}
	id := strings.Trim(path[i+len("/qrisgate/"):], "/")
	return id, uuidRe.MatchString(id)
}

// VerifyWebhook authenticates a notification from the merchant's listener (X-Listener-Timestamp, X-Listener-Signature
// = HMAC over timestamp.rawBody with the merchant's listener secret, 5 minute skew, constant-time compare; an optional
// "sha256=" prefix is accepted) and matches it to a payment. It returns a PAID event for exactly one payment whose pay
// amount equals the notification's amount; anything else (test, unmatched, ignored, replay of an unmatched one) comes
// back with no PaymentRef, which the caller acknowledges without changing a payment.
func (a *Adapter) VerifyWebhook(h http.Header, path string, body []byte) (gateway.WebhookEvent, error) {
	bad := func(reason string) (gateway.WebhookEvent, error) {
		return gateway.WebhookEvent{}, &gateway.SignatureError{Reason: reason}
	}
	merchantID, ok := merchantFromPath(path)
	if !ok {
		return bad("no_merchant_id_in_path")
	}
	rawSig := h.Get("X-Listener-Signature")
	if rawSig == "" {
		return bad("missing_signature_header")
	}
	ts, err := strconv.ParseInt(h.Get("X-Listener-Timestamp"), 10, 64)
	if err != nil {
		return bad("missing_or_bad_timestamp_header")
	}
	if d := a.cfg.Now().Unix() - ts; d > int64(MaxSkew/time.Second) || d < -int64(MaxSkew/time.Second) {
		return bad("timestamp_skew:" + strconv.FormatInt(d, 10) + "s")
	}
	secret, err := a.cfg.Port.ListenerSecret(context.Background(), merchantID)
	if err != nil || secret == "" {
		return bad("no_listener_secret_for_merchant")
	}
	sig := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(rawSig)), "sha256=")
	if !hmac.Equal([]byte(Signature(secret, ts, body)), []byte(sig)) {
		return bad("signature_mismatch")
	}
	var n notification
	if json.Unmarshal(body, &n) != nil {
		return gateway.WebhookEvent{}, errors.New("qrisgate listener: unusable payload")
	}
	if n.Event == "listener.test" {
		return gateway.WebhookEvent{RawStatus: "TEST", MerchantID: merchantID}, nil
	}
	delivery := h.Get("X-Listener-Delivery-Id")
	if delivery == "" {
		delivery = n.DeliveryID
	}
	if delivery == "" || n.Package == "" {
		return gateway.WebhookEvent{}, errors.New("qrisgate listener: missing delivery id or package")
	}
	posted, err := time.Parse(time.RFC3339, n.PostedAt)
	if err != nil {
		posted = a.cfg.Now() // a device with a broken clock must not be able to widen the matching window
	}
	var amount int64
	if n.Amount != nil {
		amount = *n.Amount
	}
	pid, base, matched, err := a.cfg.Port.MatchNotification(context.Background(), merchantID, delivery, n.Package, n.Title, n.Text, amount, posted)
	if err != nil {
		return gateway.WebhookEvent{}, err // transient (database): the caller answers 5xx and the listener retries
	}
	if !matched {
		return gateway.WebhookEvent{RawStatus: "UNMATCHED", MerchantID: merchantID}, nil
	}
	st, ok := a.MapStatus("PAID")
	return gateway.WebhookEvent{PaymentRef: pid, RawStatus: "PAID", Status: st, StatusOK: ok, Amount: base, Currency: "IDR",
		DedupKey: Code + ":listener:" + delivery, MerchantID: merchantID}, nil
}

// TestWebhook checks, without any network call, that the merchant's listener secret works: it signs a test
// notification with the stored secret exactly like the listener does and verifies it. The secret is never returned.
func (a *Adapter) TestWebhook(ctx context.Context, merchantID string) []gateway.ConnectionStep {
	if !uuidRe.MatchString(merchantID) {
		return []gateway.ConnectionStep{{Name: "secret", Code: "bad_merchant", Message: "invalid merchant id"}}
	}
	secret, err := a.cfg.Port.ListenerSecret(ctx, merchantID)
	if err != nil || secret == "" {
		return []gateway.ConnectionStep{{Name: "secret", Code: "no_secret", Message: "no listener secret yet"}}
	}
	steps := []gateway.ConnectionStep{{Name: "secret", OK: true}}
	now := a.cfg.Now()
	body := []byte(`{"event":"listener.test","delivery_id":"erapay-self-test","package":"erapay.test","title":"Tes","text":"Rp 10.000 masuk (tes)"}`)
	h := http.Header{}
	h.Set("X-Listener-Timestamp", strconv.FormatInt(now.Unix(), 10))
	h.Set("X-Listener-Signature", "sha256="+Signature(secret, now.Unix(), body))
	h.Set("X-Listener-Delivery-Id", "erapay-self-test")
	if _, err := a.VerifyWebhook(h, "/webhooks/qrisgate/"+merchantID, body); err != nil {
		return append(steps, gateway.ConnectionStep{Name: "signature", Code: "verify_failed", Message: "a test notification signed with the stored secret did not verify"})
	}
	return append(steps, gateway.ConnectionStep{Name: "signature", OK: true})
}

var _ gateway.WebhookVerifier = (*Adapter)(nil)
