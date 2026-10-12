package doku

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// Inbound QRIS notifications use DOKU's non-SNAP HTTP notification format, observed from a
// real sandbox payment (doku.md §4): headers Client-Id, Request-Id, Request-Timestamp,
// Signature=HMACSHA256=...; JSON body with order.invoice_number (= our payment id),
// order.amount (decimal number) and transaction.status. No Authorization header.

// WebhookSignature = "HMACSHA256=" + base64(HMAC-SHA256(secret, components)) where components are
// joined by "\n" (no trailing newline):
//
//	Client-Id:<..>  Request-Id:<..>  Request-Timestamp:<..>  Request-Target:<path>  Digest:<base64(sha256(body))>
func WebhookSignature(secret, clientID, requestID, timestamp, target string, body []byte) string {
	d := sha256.Sum256(body)
	comp := strings.Join([]string{
		"Client-Id:" + clientID,
		"Request-Id:" + requestID,
		"Request-Timestamp:" + timestamp,
		"Request-Target:" + target,
		"Digest:" + base64.StdEncoding.EncodeToString(d[:]),
	}, "\n")
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(comp))
	return "HMACSHA256=" + base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// webhookStatusTable: only statuses actually observed are mapped. Anything else is
// recorded but never applied (the inquiry poller still decides).
var webhookStatusTable = map[string]gateway.Status{
	"SUCCESS": gateway.StatusSucceeded,
}

type notification struct {
	Order struct {
		InvoiceNumber string      `json:"invoice_number"`
		Amount        json.Number `json:"amount"`
	} `json:"order"`
	Transaction struct {
		Status string `json:"status"`
	} `json:"transaction"`
}

// VerifyWebhook authenticates (signature over the exact raw body) and parses a notification.
// Replays are harmless: the dedup key is unique per Request-Id and applying is idempotent.
func (a *Adapter) VerifyWebhook(h http.Header, path string, body []byte) (gateway.WebhookEvent, error) {
	cid, rid, rts, sig := h.Get("Client-Id"), h.Get("Request-Id"), h.Get("Request-Timestamp"), h.Get("Signature")
	if cid == "" || rid == "" || rts == "" || sig == "" || cid != a.cfg.ClientID {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}
	want := WebhookSignature(a.cfg.ClientSecret, cid, rid, rts, path, body)
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return gateway.WebhookEvent{}, gateway.ErrInvalidSignature
	}

	var n notification
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // never float: the amount is money
	if err := dec.Decode(&n); err != nil {
		return gateway.WebhookEvent{}, fmt.Errorf("doku webhook: malformed body: %w", err)
	}
	amt, err := wholeAmount(n.Order.Amount.String())
	if err != nil {
		return gateway.WebhookEvent{}, err
	}
	if n.Order.InvoiceNumber == "" {
		return gateway.WebhookEvent{}, errors.New("doku webhook: missing order.invoice_number")
	}
	st, ok := webhookStatusTable[n.Transaction.Status]
	return gateway.WebhookEvent{
		PaymentRef: n.Order.InvoiceNumber,
		RawStatus:  n.Transaction.Status,
		Status:     st, StatusOK: ok,
		Amount: amt, Currency: "IDR", // QRIS is IDR-only; the payload carries no currency
		DedupKey: Code + ":webhook:" + rid,
	}, nil
}

// wholeAmount converts a decimal like "1000.00" to 1000. IDR has no minor unit here, so a
// non-zero fraction is rejected rather than rounded.
func wholeAmount(s string) (int64, error) {
	whole, frac, _ := strings.Cut(s, ".")
	if strings.Trim(frac, "0") != "" || whole == "" || strings.ContainsAny(whole, "eE+-") {
		return 0, fmt.Errorf("doku webhook: unsupported amount %q", s)
	}
	var v int64
	for _, c := range whole {
		if c < '0' || c > '9' || v > (1<<62)/10 {
			return 0, fmt.Errorf("doku webhook: unsupported amount %q", s)
		}
		v = v*10 + int64(c-'0')
	}
	return v, nil
}

var _ gateway.WebhookVerifier = (*Adapter)(nil)
