package ids

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// maxSkew is the replay window IDS documents for X-IDS-Timestamp.
const maxSkew = 300 * time.Second

// CallbackSignature = hex(HMAC-SHA256(callback_secret, "{timestamp}.{raw body}")).
func CallbackSignature(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp))
	m.Write([]byte("."))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

type callback struct {
	PaymentID     string      `json:"payment_id"`
	ChannelID     string      `json:"income_channel_id"`
	TransactionID string      `json:"income_channel_transaction_id"`
	Status        string      `json:"status"`
	Amount        json.Number `json:"amount"`
	PaidAmount    json.Number `json:"paid_amount"`
	Currency      string      `json:"currency"`
}

// VerifyWebhook authenticates (HMAC over the exact raw body, 5 minute skew) and parses an IDS
// callback. Replays and retries are harmless: the dedup key is (payment_id, status) as IDS
// advises, and applying a status is idempotent and never regresses.
func (a *Adapter) VerifyWebhook(h http.Header, _ string, body []byte) (gateway.WebhookEvent, error) {
	tsHeader, sig := h.Get("X-IDS-Timestamp"), h.Get("X-IDS-Signature")
	if tsHeader == "" || sig == "" {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	sec, err := strconv.ParseInt(tsHeader, 10, 64)
	if err != nil {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	if d := a.cfg.Now().Sub(time.Unix(sec, 0)); d > maxSkew || d < -maxSkew {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	want := CallbackSignature(a.cfg.CallbackSecret, tsHeader, body)
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}

	var c callback
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // never float: the amount is money
	if err := dec.Decode(&c); err != nil {
		return gateway.WebhookEvent{}, fmt.Errorf("ids webhook: malformed body: %w", err)
	}
	if c.ChannelID != "" && c.ChannelID != a.channelID {
		return gateway.WebhookEvent{}, errors.New("ids webhook: income_channel_id does not match this channel")
	}
	if c.TransactionID == "" || c.PaymentID == "" {
		return gateway.WebhookEvent{}, errors.New("ids webhook: missing payment reference")
	}
	st, ok := a.MapStatus(c.Status)

	// The amount compared against our payment: what was actually paid on PAID, else the charge.
	amtStr := c.Amount.String()
	if c.Status == "PAID" && c.PaidAmount.String() != "" {
		amtStr = c.PaidAmount.String()
	}
	amt, err := wholeAmount(amtStr)
	if err != nil {
		return gateway.WebhookEvent{}, err
	}
	cur := c.Currency
	if cur == "" {
		cur = "IDR"
	}
	return gateway.WebhookEvent{
		PaymentRef: c.TransactionID, // our payment id
		RawStatus:  c.Status,
		Status:     st, StatusOK: ok,
		Amount: amt, Currency: cur,
		DedupKey: Code + ":webhook:" + c.PaymentID + ":" + c.Status,
	}, nil
}

// wholeAmount accepts only whole rupiah; a fraction is rejected rather than rounded.
func wholeAmount(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("ids webhook: missing amount")
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("ids webhook: unsupported amount %q", s)
	}
	return v, nil
}

var _ gateway.WebhookVerifier = (*Adapter)(nil)
