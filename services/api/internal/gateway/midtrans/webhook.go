package midtrans

import (
	"bytes"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// Signature is hex(SHA512(order_id + status_code + gross_amount + key)), gross_amount exactly as the
// body spells it ("10000.00"). Confirmed on the sandbox against GET status responses (pending and cancel);
// not yet against a real notification, whose key is assumed to be the same.
func Signature(orderID, statusCode, grossAmount, key string) string {
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + key))
	return hex.EncodeToString(sum[:])
}

type notification struct {
	OrderID       string `json:"order_id"`
	TransactionID string `json:"transaction_id"`
	StatusCode    string `json:"status_code"`
	Status        string `json:"transaction_status"`
	FraudStatus   string `json:"fraud_status"`
	GrossAmount   string `json:"gross_amount"`
	Currency      string `json:"currency"`
	Signature     string `json:"signature_key"`
}

// VerifyWebhook authenticates (signature over order_id, status_code, gross_amount) and parses a Midtrans
// notification. The signature does not cover transaction_status, so success is only accepted when the
// signed status_code is 200; an unsigned "settlement" next to another code stays unmapped. The caller
// still never trusts more than the status machine allows (replays are idempotent, status never regresses).
// There is no timestamp, so there is no skew check; DedupKey is (transaction_id, status).
func (a *Adapter) VerifyWebhook(_ http.Header, _ string, body []byte) (gateway.WebhookEvent, error) {
	if a.cfg.SignatureKey == "" {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	var n notification
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&n); err != nil {
		// Unparseable and unauthenticated: indistinguishable from a forgery.
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	if n.OrderID == "" || n.StatusCode == "" || n.GrossAmount == "" || n.Signature == "" {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	want := Signature(n.OrderID, n.StatusCode, n.GrossAmount, a.cfg.SignatureKey)
	if subtle.ConstantTimeCompare([]byte(want), []byte(n.Signature)) != 1 {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	if n.TransactionID == "" || n.Status == "" {
		return gateway.WebhookEvent{}, errors.New("midtrans webhook: missing transaction reference or status")
	}
	amt, err := wholeRupiah(n.GrossAmount)
	if err != nil {
		return gateway.WebhookEvent{}, fmt.Errorf("midtrans webhook: unsupported amount %q", n.GrossAmount)
	}
	raw := n.Status
	if raw == "settlement" && (n.StatusCode != "200" || (n.FraudStatus != "" && n.FraudStatus != "accept")) {
		raw = "unconfirmed-settlement"
	}
	st, ok := a.MapStatus(raw)
	cur := n.Currency
	if cur == "" {
		cur = "IDR"
	}
	return gateway.WebhookEvent{
		PaymentRef: n.OrderID, // our payment id
		RawStatus:  raw,
		Status:     st, StatusOK: ok,
		Amount: amt, Currency: cur,
		DedupKey: Code + ":webhook:" + n.TransactionID + ":" + n.Status,
	}, nil
}

var _ gateway.WebhookVerifier = (*Adapter)(nil)
