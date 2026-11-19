// Package xendit is the adapter for Xendit Payments API v3 (payment_requests), QRIS only. Auth is HTTP
// Basic with the secret API key as username and an empty password. The response carries Xendit's own id
// (payment_request_id), which is what inquiry needs, so it is returned as GatewayReference.
//
// Not implemented (see docs/integrations/xendit.md): webhooks (x-callback-token only proves origin, so
// status moves through inquiry), cancel, refund, payouts, settlement.
package xendit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const (
	Code = "xendit"

	// Sandbox and production share one host; the key's mode (test/live) decides.
	BaseURL = "https://api.xendit.co"

	apiVersion  = "2024-11-11"
	pathRequest = "/v3/payment_requests"

	methodQRIS = "QRIS"
)

type Config struct {
	BaseURL   string
	SecretKey string
	HTTP      *http.Client
	Now       func() time.Time
}

type Adapter struct{ cfg Config }

func New(cfg Config) (*Adapter, error) {
	if cfg.SecretKey == "" {
		return nil, errors.New("xendit: secret key is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = BaseURL
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("xendit: base URL must be an absolute http(s) URL")
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
	// One Xendit account for the whole platform; no refund/cancel/payout/settlement is advertised.
	return gateway.Capabilities{PaymentMethods: []string{methodQRIS}, Inquiry: true, PlatformAccount: true}
}

type response struct {
	ID            string      `json:"payment_request_id"`
	Status        string      `json:"status"`
	RequestAmount json.Number `json:"request_amount"`
	Currency      string      `json:"currency"`
	Updated       string      `json:"updated"`
	ErrorCode     string      `json:"error_code"`
	Message       string      `json:"message"`
	Actions       []struct {
		Descriptor string `json:"descriptor"`
		Value      string `json:"value"`
	} `json:"actions"`
}

// do sends one request. A transport failure after the request left is Ambiguous (Xendit may have
// created the QR); a create has no id to look up then, see GetPayment.
func (a *Adapter) do(ctx context.Context, method, path string, payload any) (response, int, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return response{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "encode request"}
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.cfg.BaseURL+path, body)
	if err != nil {
		return response{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "build request"}
	}
	req.SetBasicAuth(a.cfg.SecretKey, "")
	req.Header.Set("api-version", apiVersion)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.cfg.HTTP.Do(req)
	if err != nil {
		return response{}, 0, &gateway.Error{Kind: gateway.Ambiguous, Retryable: true, Message: "no response from gateway"}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out response
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // money: never float
	if dec.Decode(&out) != nil && resp.StatusCode >= 400 {
		out.Message = "respons bukan JSON dari Xendit"
	}
	return out, resp.StatusCode, classify(resp.StatusCode, out)
}

// classify: 2xx = accepted. 408, 409 (DUPLICATE_ERROR), 429, 5xx (incl. 503 CHANNEL_UNAVAILABLE) leave
// the outcome unknown. Other 4xx (400 validation, 401 key, 403 settings, 404) are definitive.
func classify(status int, r response) error {
	if status >= 200 && status < 300 {
		return nil
	}
	switch {
	case status == http.StatusRequestTimeout, status == http.StatusConflict, status == http.StatusTooManyRequests, status >= 500:
		return &gateway.Error{Kind: gateway.Ambiguous, Code: r.ErrorCode, HTTPStatus: status, Retryable: true, Message: "gateway outcome unknown"}
	default:
		return &gateway.Error{Kind: gateway.Rejected, Code: r.ErrorCode, HTTPStatus: status, Message: safeMsg(r.Message)}
	}
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

func (a *Adapter) CreatePayment(ctx context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	if r.Method != methodQRIS {
		return gateway.CreateResult{}, gateway.ErrNotSupported
	}
	if r.Reference == "" || len(r.Reference) > 255 || r.Amount <= 0 || r.Currency != "IDR" || r.ExpiresAt.IsZero() {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	if !r.ExpiresAt.After(a.cfg.Now()) {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "expiry is in the past"}
	}
	payload := map[string]any{
		"reference_id":   r.Reference,
		"type":           "PAY",
		"country":        "ID",
		"currency":       "IDR",
		"request_amount": r.Amount,
		"channel_code":   "QRIS",
		"channel_properties": map[string]string{
			"expires_at": r.ExpiresAt.UTC().Format(time.RFC3339),
		},
	}
	res, status, err := a.do(ctx, http.MethodPost, pathRequest, payload)
	if err != nil {
		return gateway.CreateResult{}, err
	}
	qr := ""
	for _, ac := range res.Actions {
		if ac.Descriptor == "QR_STRING" {
			qr = ac.Value
		}
	}
	if res.ID == "" || qr == "" {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "incomplete create response"}
	}
	return gateway.CreateResult{GatewayReference: res.ID, QRPayload: qr, RawStatus: res.Status}, nil
}

// GetPayment needs Xendit's own id. A create that ended Ambiguous never stored one, and the list
// endpoint (by reference_id) is unconfirmed, so that case stays Ambiguous until the payment expires.
func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	if r.GatewayReference == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, Message: "no xendit payment request id stored"}
	}
	if strings.ContainsAny(r.GatewayReference, "/?#") {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid query request"}
	}
	res, status, err := a.do(ctx, http.MethodGet, pathRequest+"/"+url.PathEscape(r.GatewayReference), nil)
	if err != nil {
		return gateway.QueryResult{}, err
	}
	if res.Status == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "status response without status"}
	}
	out := gateway.QueryResult{RawStatus: res.Status, Amount: -1, Currency: res.Currency}
	if v, err := strconv.ParseInt(res.RequestAmount.String(), 10, 64); err == nil && v > 0 {
		out.Amount = v
	}
	// shortcut: `updated` is the last update, not the payment time; only trusted once SUCCEEDED.
	if t, err := time.Parse(time.RFC3339, res.Updated); err == nil && res.Status == "SUCCEEDED" {
		out.PaidAt = &t
	}
	return out, nil
}

func (a *Adapter) CancelPayment(context.Context, gateway.CancelRequest) error {
	return gateway.ErrNotSupported
}

// AUTHORIZED (cards) stays unmapped on purpose, so callers leave the payment unchanged.
var statusTable = map[string]gateway.Status{
	"REQUIRES_ACTION":    gateway.StatusPending,
	"ACCEPTING_PAYMENTS": gateway.StatusPending,
	"SUCCEEDED":          gateway.StatusSucceeded,
	"EXPIRED":            gateway.StatusExpired,
	"FAILED":             gateway.StatusFailed,
	"CANCELED":           gateway.StatusCancelled,
}

func (a *Adapter) MapStatus(raw string) (gateway.Status, bool) {
	s, ok := statusTable[raw]
	return s, ok
}

var _ gateway.Adapter = (*Adapter)(nil)
