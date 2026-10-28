// Package ids is the adapter for IDS Payment Service (payment.inodreamstudio.com), an upstream
// payment API that fronts DOKU. EraPay is an `income_channel` of IDS: it creates charges with
// POST /v1/api/payments (idempotent per income_channel_transaction_id = our payment id), reads them
// with GET, and receives HMAC-signed callbacks. IDS marks a payment PAID only after it has
// confirmed it with DOKU, so a verified PAID callback is an allowed status source (PRD §0 #2).
//
// Not offered by IDS (so not advertised here): cancel, refund, payouts.
package ids

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const (
	Code = "ids"

	pathPayments = "/v1/api/payments"
	pathMethods  = "/v1/api/payment-methods"

	// MethodQRIS is EraPay's method name; IDS spells it lower case.
	methodQRIS    = "QRIS"
	idsMethodQRIS = "qris"
)

type Config struct {
	BaseURL        string // https://<ids host>, no trailing slash needed
	APIKey         string // ids_<env>_<income_channel_id>_<secret>
	CallbackSecret string // HMAC key IDS signs callbacks with
	HTTP           *http.Client
	Now            func() time.Time
}

type Adapter struct {
	cfg       Config
	channelID string
}

// ParseAPIKey extracts the income_channel_id from an IDS key (ids_<env>_<income_channel_id>_<secret>).
func ParseAPIKey(key string) (channelID string, err error) {
	parts := strings.SplitN(key, "_", 4)
	if len(parts) != 4 || parts[0] != "ids" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return "", errors.New("ids: API key must look like ids_<env>_<income_channel_id>_<secret>")
	}
	return parts[2], nil
}

func New(cfg Config) (*Adapter, error) {
	ch, err := ParseAPIKey(cfg.APIKey)
	if err != nil {
		return nil, err
	}
	if len(cfg.CallbackSecret) < 8 {
		return nil, errors.New("ids: callback secret must be at least 8 characters")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("ids: base URL must be an absolute http(s) URL")
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Adapter{cfg: cfg, channelID: ch}, nil
}

func (a *Adapter) Code() string { return Code }

func (a *Adapter) Capabilities() gateway.Capabilities {
	// IDS has no cancel, refund, payout or settlement-report API: none of them is advertised.
	return gateway.Capabilities{PaymentMethods: []string{methodQRIS}, Inquiry: true, PlatformAccount: true}
}

// ---- transport ----

type paymentDTO struct {
	PaymentID         string `json:"payment_id"`
	TransactionID     string `json:"income_channel_transaction_id"`
	Amount            int64  `json:"amount"`
	PaidAmount        int64  `json:"paid_amount"`
	Currency          string `json:"currency"`
	Status            string `json:"status"`
	QRString          string `json:"qr_string"`
	PaidAt            string `json:"paid_at"`
	ProviderReference string `json:"provider_reference_no"`
}

type envelope struct {
	Code    int         `json:"code"`
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Payment *paymentDTO `json:"payment"`
}

// do sends one request and classifies the outcome (see gateway.Kind). A transport failure after
// the request left is Ambiguous: IDS may have created the charge. Create is idempotent on our
// payment id, so the caller resolves ambiguity by inquiry and never re-creates blindly.
func (a *Adapter) do(ctx context.Context, method, path string, query url.Values, payload any) (envelope, int, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "encode request"}
		}
		body = bytes.NewReader(b)
	}
	u := a.cfg.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "build request"}
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
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
	var env envelope
	if json.Unmarshal(raw, &env) != nil && resp.StatusCode >= 400 {
		// An HTML or empty error page means we did not reach the IDS API at all (typically a base URL
		// that points at the dashboard). Say so: the status code alone is a baffling 404.
		env.Message = "respons bukan JSON dari IDS: periksa Base URL (harus host API, bukan dashboard)"
	}
	return env, resp.StatusCode, classify(resp.StatusCode, env)
}

// classify: 2xx = accepted. 409 (IDS already holds this reference), 408, 429 and 5xx (including
// 502 CREATING) mean the outcome is unknown. Other 4xx are definitive rejections.
func classify(status int, env envelope) error {
	code := ""
	if env.Code != 0 {
		code = itoa(env.Code)
	}
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusConflict, status == http.StatusRequestTimeout, status == http.StatusTooManyRequests, status >= 500:
		return &gateway.Error{Kind: gateway.Ambiguous, Code: code, HTTPStatus: status, Retryable: true, Message: "gateway outcome unknown"}
	default:
		return &gateway.Error{Kind: gateway.Rejected, Code: code, HTTPStatus: status, Message: safeMsg(env.Message)}
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func safeMsg(s string) string {
	if len(s) > 120 {
		s = s[:120]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// ---- operations ----

func (a *Adapter) CreatePayment(ctx context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	if r.Method != methodQRIS {
		return gateway.CreateResult{}, gateway.ErrNotSupported
	}
	if r.Reference == "" || len(r.Reference) > 64 || r.Amount <= 0 || r.Currency != "IDR" || r.ExpiresAt.IsZero() {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	ttl := int64(r.ExpiresAt.Sub(a.cfg.Now()).Seconds())
	if ttl <= 0 {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "expiry is in the past"}
	}
	payload := map[string]any{
		"income_channel_id":             a.channelID,
		"income_channel_transaction_id": r.Reference,
		"payment_method":                idsMethodQRIS,
		"currency":                      r.Currency,
		"amount":                        r.Amount,
		"expiry_seconds":                ttl,
	}
	env, status, err := a.do(ctx, http.MethodPost, pathPayments, nil, payload)
	if err != nil {
		return gateway.CreateResult{}, err
	}
	if env.Payment == nil || env.Payment.PaymentID == "" || env.Payment.QRString == "" {
		// A "success" without the data we need: we cannot tell what IDS did.
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "incomplete create response"}
	}
	return gateway.CreateResult{GatewayReference: env.Payment.PaymentID, QRPayload: env.Payment.QRString,
		RawStatus: env.Payment.Status, ResponseCode: itoa(env.Code)}, nil
}

// GetPayment reads IDS's state (it does not query DOKU). It is looked up by our payment id, so it
// works even when we never saw IDS's payment_id.
func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	if r.Reference == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid query request"}
	}
	q := url.Values{"income_channel_transaction_id": {r.Reference}}
	env, _, err := a.do(ctx, http.MethodGet, pathPayments, q, nil)
	if err != nil {
		return gateway.QueryResult{}, err
	}
	p := env.Payment
	if p == nil || p.Status == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: 200, Message: "status response without payment"}
	}
	res := gateway.QueryResult{RawStatus: p.Status, Amount: -1, Currency: p.Currency, ResponseCode: itoa(env.Code)}
	switch {
	case p.Status == "PAID" && p.PaidAmount > 0:
		res.Amount = p.PaidAmount
	case p.Amount > 0:
		res.Amount = p.Amount
	}
	if t, err := time.Parse(time.RFC3339, p.PaidAt); err == nil {
		res.PaidAt = &t
	}
	return res, nil
}

// CancelPayment: IDS exposes no cancel; an unpaid QR simply expires.
func (a *Adapter) CancelPayment(context.Context, gateway.CancelRequest) error {
	return gateway.ErrNotSupported
}

// statusTable maps the documented IDS statuses. CREATING is internal and SETTLED is reserved and
// not emitted for QRIS, so both stay unmapped: callers leave the payment unchanged.
var statusTable = map[string]gateway.Status{
	"PENDING": gateway.StatusPending,
	"PAID":    gateway.StatusSucceeded,
	"EXPIRED": gateway.StatusExpired,
	"FAILED":  gateway.StatusFailed,
}

func (a *Adapter) MapStatus(raw string) (gateway.Status, bool) {
	s, ok := statusTable[raw]
	return s, ok
}

var _ gateway.Adapter = (*Adapter)(nil)
