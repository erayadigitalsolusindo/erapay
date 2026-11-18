// Package finpay is the adapter for FINPAY (Finnet) Core API, QRIS only. Auth is HTTP Basic with
// merchant_id:merchant_key. Create is idempotent on order.id (= our payment id): a repeat answers 409
// "Duplicate partnerReferenceNo" with the original QR, which we treat as success.
//
// Not implemented (see docs/integrations/finpay.md): callbacks (the signature's JSON serialization is
// undocumented, so payment status moves only through inquiry), cancel, refund, payouts, settlement.
package finpay

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
	Code = "finpay"

	SandboxURL    = "https://devo.finnet.co.id"
	ProductionURL = "https://live.finnet.co.id"

	pathInitiate = "/pg/payment/card/initiate"
	pathCheck    = "/pg/payment/card/check/"

	methodQRIS = "QRIS"

	rcOK        = "2000000"
	rcDuplicate = "4090001"

	// FINPAY rejects order.timeout below 6 minutes (sandbox: "must be greater than 5").
	minTimeoutMin = 6
)

// wib is the zone of FINPAY's timestamps (expiryLink came back in UTC+7 in the sandbox).
var wib = time.FixedZone("WIB", 7*3600)

type Config struct {
	BaseURL     string // SandboxURL or ProductionURL
	MerchantID  string
	MerchantKey string
	CallbackURL string // FINPAY requires url.callbackUrl on create; EraPay serves /webhooks/finpay
	HTTP        *http.Client
	Now         func() time.Time
}

type Adapter struct{ cfg Config }

func New(cfg Config) (*Adapter, error) {
	if cfg.MerchantID == "" || cfg.MerchantKey == "" {
		return nil, errors.New("finpay: merchant id and key are required")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("finpay: base URL must be an absolute http(s) URL")
	}
	if cu, err := url.Parse(cfg.CallbackURL); err != nil || cu.Host == "" || cu.Scheme != "https" {
		return nil, errors.New("finpay: callback URL must be an absolute https URL")
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
	// One merchant id for the whole platform; no refund/cancel/payout/settlement is advertised.
	return gateway.Capabilities{PaymentMethods: []string{methodQRIS}, Inquiry: true, PlatformAccount: true}
}

type envelope struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	StringQR        string `json:"stringQr"`
	ExpiryLink      string `json:"expiryLink"`
	Data            *struct {
		Order struct {
			Amount   json.Number `json:"amount"`
			Currency string      `json:"currency"`
		} `json:"order"`
		Result struct {
			Payment struct {
				Status   string      `json:"status"`
				Amount   json.Number `json:"amount"`
				Datetime string      `json:"datetime"`
			} `json:"payment"`
		} `json:"result"`
	} `json:"data"`
}

// do sends one request. A transport failure after the request left is Ambiguous (FINPAY may have
// created the QR); the caller resolves it by inquiry, never by creating again with a new id.
func (a *Adapter) do(ctx context.Context, method, path string, payload any) (envelope, int, error) {
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
	req.SetBasicAuth(a.cfg.MerchantID, a.cfg.MerchantKey)
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
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // money: never float
	if dec.Decode(&env) != nil && resp.StatusCode >= 400 {
		env.ResponseMessage = "respons bukan JSON dari FINPAY"
	}
	return env, resp.StatusCode, classify(resp.StatusCode, env)
}

// classify: 2xx with responseCode 2000000 = accepted. 408, 409, 429, 5xx and 2xx with another code
// leave the outcome unknown. Other 4xx (400 bad field, 401 bad key, 404 not found) are definitive.
func classify(status int, env envelope) error {
	if status >= 200 && status < 300 && env.ResponseCode == rcOK {
		return nil
	}
	switch {
	case status == http.StatusRequestTimeout, status == http.StatusConflict, status == http.StatusTooManyRequests,
		status >= 500, status >= 200 && status < 300:
		return &gateway.Error{Kind: gateway.Ambiguous, Code: env.ResponseCode, HTTPStatus: status, Retryable: true, Message: "gateway outcome unknown"}
	default:
		return &gateway.Error{Kind: gateway.Rejected, Code: env.ResponseCode, HTTPStatus: status, Message: safeMsg(env.ResponseMessage)}
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
	if r.Reference == "" || len(r.Reference) > 64 || r.Amount <= 0 || r.Currency != "IDR" || r.ExpiresAt.IsZero() {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	ttl := r.ExpiresAt.Sub(a.cfg.Now())
	if ttl <= 0 {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "expiry is in the past"}
	}
	// shortcut: a QR shorter than FINPAY's 6 minute minimum lives a little longer at FINPAY than at
	// EraPay; EraPay expires the payment first, so a late scan is a mismatch for reconciliation.
	timeout := max(int((ttl+time.Minute-1)/time.Minute), minTimeoutMin)
	desc := r.Description
	if desc == "" {
		desc = "EraPay"
	}
	payload := map[string]any{
		// FINPAY requires a customer; the payer is anonymous to us, so no personal data is sent.
		"customer": map[string]string{"email": "payer@example.com", "firstName": "EraPay", "lastName": "Payer", "mobilePhone": "+6281200000000"},
		"order": map[string]any{"id": r.Reference, "amount": strconv.FormatInt(r.Amount, 10),
			"description": safeMsg(desc), "timeout": timeout},
		"url":           map[string]string{"callbackUrl": a.cfg.CallbackURL},
		"sourceOfFunds": map[string]string{"type": "qris"},
	}
	env, status, err := a.do(ctx, http.MethodPost, pathInitiate, payload)
	// Duplicate = this order id was already accepted; FINPAY returns the original QR.
	if err != nil && env.ResponseCode == rcDuplicate && env.StringQR != "" {
		err = nil
	}
	if err != nil {
		return gateway.CreateResult{}, err
	}
	if env.StringQR == "" {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "incomplete create response"}
	}
	return gateway.CreateResult{GatewayReference: r.Reference, QRPayload: env.StringQR, RawStatus: "PENDING", ResponseCode: env.ResponseCode}, nil
}

func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	if r.Reference == "" || len(r.Reference) > 64 || strings.ContainsAny(r.Reference, "/?#") {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid query request"}
	}
	env, status, err := a.do(ctx, http.MethodGet, pathCheck+url.PathEscape(r.Reference), nil)
	if err != nil {
		return gateway.QueryResult{}, err
	}
	if env.Data == nil || env.Data.Result.Payment.Status == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "status response without payment"}
	}
	p := env.Data.Result.Payment
	res := gateway.QueryResult{RawStatus: p.Status, Amount: -1, Currency: env.Data.Order.Currency, ResponseCode: env.ResponseCode}
	// Pending shows payment.amount 0; the paid amount only counts once paid.
	if v, err := strconv.ParseInt(p.Amount.String(), 10, 64); err == nil && v > 0 && p.Status == "PAID" {
		res.Amount = v
	} else if v, err := strconv.ParseInt(env.Data.Order.Amount.String(), 10, 64); err == nil && v > 0 {
		res.Amount = v
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", p.Datetime, wib); err == nil {
		res.PaidAt = &t
	}
	return res, nil
}

// CancelPayment: FINPAY has a cancel-order page we have not confirmed for QRIS; an unpaid QR expires.
func (a *Adapter) CancelPayment(context.Context, gateway.CancelRequest) error {
	return gateway.ErrNotSupported
}

// statusTable holds the documented statuses relevant to QRIS. Only PENDING has been observed in the
// sandbox so far; card statuses (AUTHORIZED, CAPTURED, ...) and REFUNDED stay unmapped on purpose, so
// callers leave the payment unchanged.
var statusTable = map[string]gateway.Status{
	"PENDING":   gateway.StatusPending,
	"PAID":      gateway.StatusSucceeded,
	"EXPIRED":   gateway.StatusExpired,
	"FAILURE":   gateway.StatusFailed,
	"CANCELLED": gateway.StatusCancelled,
}

func (a *Adapter) MapStatus(raw string) (gateway.Status, bool) {
	s, ok := statusTable[raw]
	return s, ok
}

var _ gateway.Adapter = (*Adapter)(nil)
