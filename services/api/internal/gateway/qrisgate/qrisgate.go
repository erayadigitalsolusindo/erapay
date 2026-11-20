// Package qrisgate is the adapter for QRISGate (qrisgate.web.id): a QRIS gateway that detects payments from the
// merchant's own bank/e-wallet mutations. The money goes straight to the merchant, so it is a Direct gateway: no
// clearing, settlement, refund or payout (see docs/integrations/qrisgate.md). Credentials are PER MERCHANT (their
// own API key and webhook secret), resolved through CredentialSource; AccountRef is the merchant id.
//
// The API shape comes from the public docs as read on 20 Nov 2026 and is NOT yet confirmed against a real account:
// the status values, the GET auth and the error body are assumptions marked in the doc; an unknown status stays
// unmapped (payment unchanged), never guessed.
package qrisgate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const (
	Code       = "qrisgate"
	DefaultURL = "https://qrisgate.web.id"
	methodQRIS = "QRIS"
)

// CredentialSource returns a merchant's QRISGate API key and webhook secret ("not configured" = an error).
type CredentialSource interface {
	Credentials(ctx context.Context, merchantID string) (apiKey, webhookSecret string, err error)
}

type Config struct {
	BaseURL string
	Creds   CredentialSource
	HTTP    *http.Client
	Now     func() time.Time
}

type Adapter struct{ cfg Config }

func New(cfg Config) (*Adapter, error) {
	if cfg.Creds == nil {
		return nil, errors.New("qrisgate: credential source is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultURL
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("qrisgate: base URL must be an absolute http(s) URL")
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 20 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Adapter{cfg: cfg}, nil
}

func (a *Adapter) Code() string { return Code }

func (a *Adapter) Capabilities() gateway.Capabilities {
	return gateway.Capabilities{PaymentMethods: []string{methodQRIS}, Inquiry: true, Direct: true}
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// envelope: responses wrap the transaction in "data".
type envelope struct {
	Data struct {
		TransactionID string `json:"transaction_id"`
		BaseAmount    int64  `json:"base_amount"`
		UniqueCode    int64  `json:"unique_code"`
		TotalAmount   int64  `json:"total_amount"`
		Status        string `json:"status"`
		QRString      string `json:"qr_string"`
		PaidAt        string `json:"paid_at"`
	} `json:"data"`
}

// do sends one request with the merchant's key. A transport failure is Ambiguous (the request may have landed);
// 408/409/429/5xx are Ambiguous too; other 4xx are definitive.
func (a *Adapter) do(ctx context.Context, merchantID, method, path string, payload any) (envelope, int, error) {
	key, _, err := a.cfg.Creds.Credentials(ctx, merchantID)
	if err != nil || key == "" {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Code: "no_credentials", Message: "QRISGate API key is not configured for this merchant"}
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "encode request"}
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.cfg.BaseURL+path, body)
	if err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "build request"}
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.cfg.HTTP.Do(req)
	if err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Ambiguous, Retryable: true, Message: "no response from gateway"}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	switch {
	case resp.StatusCode >= 500, resp.StatusCode == 408, resp.StatusCode == 409, resp.StatusCode == 429:
		return envelope{}, resp.StatusCode, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: resp.StatusCode, Retryable: true, Message: "gateway outcome unknown"}
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return envelope{}, resp.StatusCode, &gateway.Error{Kind: gateway.Rejected, HTTPStatus: resp.StatusCode, Message: "gateway rejected the request"}
	}
	var out envelope
	if json.Unmarshal(raw, &out) != nil {
		return envelope{}, resp.StatusCode, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: resp.StatusCode, Message: "response is not JSON"}
	}
	return out, resp.StatusCode, nil
}

// CreatePayment: external_order_id = our payment id (QRISGate dedups on it). The buyer pays total_amount (the
// base amount plus a small unique code), which is encoded in the QR; our payment keeps the BASE amount.
func (a *Adapter) CreatePayment(ctx context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	if r.Method != methodQRIS {
		return gateway.CreateResult{}, gateway.ErrNotSupported
	}
	if !uuidRe.MatchString(r.AccountRef) || r.Reference == "" || r.Amount <= 0 || r.Currency != "IDR" {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	res, st, err := a.do(ctx, r.AccountRef, http.MethodPost, "/api/v1/transactions", map[string]any{
		"external_order_id": r.Reference, "amount": r.Amount, "payment_method": methodQRIS})
	if err != nil {
		return gateway.CreateResult{}, err
	}
	d := res.Data
	if d.TransactionID == "" || d.QRString == "" {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: st, Message: "incomplete create response"}
	}
	if d.BaseAmount != 0 && d.BaseAmount != r.Amount {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: st, Message: "gateway amount differs from the request"}
	}
	return gateway.CreateResult{GatewayReference: d.TransactionID, QRPayload: d.QRString, RawStatus: d.Status, ResponseCode: itoa(st)}, nil
}

// GetPayment asks by the gateway's transaction id. The docs show no lookup by our order id, so a payment whose
// create never returned an id cannot be resolved here: that stays Ambiguous (it expires or is handled by hand).
func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	if !uuidRe.MatchString(r.AccountRef) {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid query request"}
	}
	if r.GatewayReference == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, Message: "no gateway reference to look up"}
	}
	res, st, err := a.do(ctx, r.AccountRef, http.MethodGet, "/api/v1/transactions/"+url.PathEscape(r.GatewayReference), nil)
	if err != nil {
		return gateway.QueryResult{}, err
	}
	if res.Data.Status == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: st, Message: "status response without status"}
	}
	out := gateway.QueryResult{RawStatus: res.Data.Status, Amount: -1, Currency: "IDR", ResponseCode: itoa(st)}
	if res.Data.BaseAmount > 0 {
		out.Amount = res.Data.BaseAmount
	}
	if t, err := time.Parse(time.RFC3339, res.Data.PaidAt); err == nil {
		out.PaidAt = &t
	}
	return out, nil
}

func (a *Adapter) CancelPayment(context.Context, gateway.CancelRequest) error {
	return gateway.ErrNotSupported
}

// The docs name only PENDING and the events payment.paid/expired/cancelled; other spellings are not guessed.
var statusTable = map[string]gateway.Status{
	"PENDING":   gateway.StatusPending,
	"PAID":      gateway.StatusSucceeded,
	"EXPIRED":   gateway.StatusExpired,
	"CANCELLED": gateway.StatusCancelled,
}

func (a *Adapter) MapStatus(raw string) (gateway.Status, bool) {
	s, ok := statusTable[strings.ToUpper(raw)]
	return s, ok
}

func itoa(i int) string {
	if i == 0 {
		return ""
	}
	return strconv.Itoa(i)
}

var _ gateway.Adapter = (*Adapter)(nil)
