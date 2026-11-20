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

// Signature is hex(HMAC-SHA256(secret, timestamp + "." + rawBody)).
func Signature(secret string, timestamp int64, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

type notification struct {
	Event       string `json:"event"`
	Transaction string `json:"transaction_id"`
	OrderID     string `json:"external_order_id"`
	BaseAmount  int64  `json:"base_amount"`
}

// merchantFromPath reads the merchant id out of /webhooks/qrisgate/<merchant_id>: each merchant gives QRISGate
// that URL, and the secret that verifies it is that merchant's own.
func merchantFromPath(path string) (string, bool) {
	i := strings.Index(path, "/qrisgate/")
	if i < 0 {
		return "", false
	}
	id := strings.Trim(path[i+len("/qrisgate/"):], "/")
	return id, uuidRe.MatchString(id)
}

// VerifyWebhook authenticates (HMAC over timestamp.rawBody with the merchant's webhook secret, 5 minute skew,
// constant-time) and parses a QRISGate notification. The signature may carry a "sha256=" prefix (not yet
// confirmed which form the real service sends, so both are accepted). DedupKey is the Delivery-Id.
func (a *Adapter) VerifyWebhook(h http.Header, path string, body []byte) (gateway.WebhookEvent, error) {
	merchantID, ok := merchantFromPath(path)
	if !ok {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	ts, err := strconv.ParseInt(h.Get("X-QRISGate-Timestamp"), 10, 64)
	sig := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(h.Get("X-QRISGate-Signature"))), "sha256=")
	if err != nil || sig == "" {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	if d := a.cfg.Now().Unix() - ts; d > int64(MaxSkew/time.Second) || d < -int64(MaxSkew/time.Second) {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	_, secret, err := a.cfg.Creds.Credentials(context.Background(), merchantID)
	if err != nil || secret == "" {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	if !hmac.Equal([]byte(Signature(secret, ts, body)), []byte(sig)) {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	var n notification
	if json.Unmarshal(body, &n) != nil || n.OrderID == "" {
		return gateway.WebhookEvent{}, errors.New("qrisgate webhook: unusable payload")
	}
	delivery := h.Get("X-QRISGate-Delivery-Id")
	if delivery == "" {
		return gateway.WebhookEvent{}, errors.New("qrisgate webhook: missing delivery id")
	}
	raw := strings.ToUpper(strings.TrimPrefix(h.Get("X-QRISGate-Event"), "payment."))
	if raw == "" {
		raw = strings.ToUpper(strings.TrimPrefix(n.Event, "payment."))
	}
	if raw == "PAID" || raw == "EXPIRED" || raw == "CANCELLED" {
		st, ok := a.MapStatus(raw)
		return gateway.WebhookEvent{PaymentRef: n.OrderID, RawStatus: raw, Status: st, StatusOK: ok,
			Amount: n.BaseAmount, Currency: "IDR", DedupKey: Code + ":webhook:" + delivery, MerchantID: merchantID}, nil
	}
	return gateway.WebhookEvent{PaymentRef: n.OrderID, RawStatus: raw, Amount: n.BaseAmount, Currency: "IDR",
		DedupKey: Code + ":webhook:" + delivery, MerchantID: merchantID}, nil
}

var _ gateway.WebhookVerifier = (*Adapter)(nil)

// TestWebhook checks, without any network call, that this merchant's stored webhook secret works: it builds a
// notification for a payment that does not exist, signs it with the stored secret exactly like QRISGate would, and
// verifies it through VerifyWebhook. It proves the credentials are saved and readable and that the signing scheme
// round-trips; it does not prove that QRISGate can reach the public URL or that its own secret matches. The
// secret itself is never returned.
func (a *Adapter) TestWebhook(ctx context.Context, merchantID string) []gateway.ConnectionStep {
	if !uuidRe.MatchString(merchantID) {
		return []gateway.ConnectionStep{{Name: "credentials", Code: "bad_merchant", Message: "invalid merchant id"}}
	}
	_, secret, err := a.cfg.Creds.Credentials(ctx, merchantID)
	if err != nil || secret == "" {
		return []gateway.ConnectionStep{{Name: "credentials", Code: "no_credentials", Message: "API key and webhook secret are not saved yet"}}
	}
	steps := []gateway.ConnectionStep{{Name: "credentials", OK: true}}
	now := a.cfg.Now()
	body := []byte(`{"event":"payment.paid","transaction_id":"listener-test","external_order_id":"00000000-0000-0000-0000-000000000000","base_amount":10000}`)
	h := http.Header{}
	h.Set("X-QRISGate-Timestamp", strconv.FormatInt(now.Unix(), 10))
	h.Set("X-QRISGate-Signature", Signature(secret, now.Unix(), body))
	h.Set("X-QRISGate-Event", "payment.paid")
	h.Set("X-QRISGate-Delivery-Id", "erapay-self-test")
	if _, err := a.VerifyWebhook(h, "/webhooks/qrisgate/"+merchantID, body); err != nil {
		return append(steps, gateway.ConnectionStep{Name: "signature", Code: "verify_failed", Message: "a test notification signed with the stored secret did not verify"})
	}
	return append(steps, gateway.ConnectionStep{Name: "signature", OK: true})
}
