// Package midtrans is the adapter for Midtrans Core API v2, QRIS only (not Snap, not BI-SNAP). Auth is
// HTTP Basic with the server key as username. Midtrans answers HTTP 200 even for errors: the real result
// is the string status_code in the body, so every call is classified on that, not on the HTTP status.
//
// Inquiry works by our own order_id (= payment id), so a create that ended Ambiguous can still be looked
// up. Not implemented (see docs/integrations/midtrans.md): cancel, refund, payouts, settlement report.
package midtrans

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
	Code = "midtrans"

	SandboxURL    = "https://api.sandbox.midtrans.com"
	ProductionURL = "https://api.midtrans.com"

	methodQRIS = "QRIS"

	// Midtrans keeps an Idempotency-Key for 5 minutes and ignores one over 46 characters.
	maxIdempotencyKey = 46
)

// orderIDPattern: Midtrans accepts only these characters in order_id.
var orderIDPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,100}$`)

type Config struct {
	BaseURL   string
	ServerKey string
	// SignatureKey is the secret behind the webhook signature_key. Confirmed on the sandbox that it is
	// NOT the SB- server key; without it VerifyWebhook rejects everything.
	SignatureKey string
	HTTP         *http.Client
	Now          func() time.Time
}

type Adapter struct{ cfg Config }

func New(cfg Config) (*Adapter, error) {
	if cfg.ServerKey == "" {
		return nil, errors.New("midtrans: server key is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxURL
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("midtrans: base URL must be an absolute http(s) URL")
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

// QRImageURL is the hosted QR image that Midtrans' payment simulator asks for; sandbox only ("" on production).
func (a *Adapter) QRImageURL(ref string) string {
	if a.cfg.BaseURL != SandboxURL {
		return ""
	}
	return a.cfg.BaseURL + "/v2/qris/" + ref + "/qr-code"
}

func (a *Adapter) Capabilities() gateway.Capabilities {
	// One Midtrans account for the whole platform; no refund/cancel/payout/settlement is advertised.
	return gateway.Capabilities{PaymentMethods: []string{methodQRIS}, Inquiry: true, PlatformAccount: true}
}

type response struct {
	StatusCode    string `json:"status_code"`
	Message       string `json:"status_message"`
	TransactionID string `json:"transaction_id"`
	OrderID       string `json:"order_id"`
	GrossAmount   string `json:"gross_amount"`
	Currency      string `json:"currency"`
	Status        string `json:"transaction_status"`
	FraudStatus   string `json:"fraud_status"`
	QRString      string `json:"qr_string"`
	SettleTime    string `json:"settlement_time"`
}

// wib is GMT+7, the zone of every Midtrans timestamp.
var wib = time.FixedZone("WIB", 7*3600)

// do sends one request and returns the decoded body. A transport failure after the request left is
// Ambiguous; otherwise the body's status_code decides (classify).
func (a *Adapter) do(ctx context.Context, method, path, idemKey string, payload any) (response, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return response{}, &gateway.Error{Kind: gateway.Rejected, Message: "encode request"}
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.cfg.BaseURL+path, body)
	if err != nil {
		return response{}, &gateway.Error{Kind: gateway.Rejected, Message: "build request"}
	}
	req.SetBasicAuth(a.cfg.ServerKey, "")
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" && len(idemKey) <= maxIdempotencyKey {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := a.cfg.HTTP.Do(req)
	if err != nil {
		return response{}, &gateway.Error{Kind: gateway.Ambiguous, Retryable: true, Message: "no response from gateway"}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out response
	if json.Unmarshal(raw, &out) != nil {
		if resp.StatusCode >= 400 {
			return response{}, classify(resp.StatusCode, response{StatusCode: strconv.Itoa(resp.StatusCode)})
		}
		return response{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: resp.StatusCode, Message: "response is not JSON"}
	}
	if out.StatusCode == "" { // an error without a body status_code: trust the HTTP status
		out.StatusCode = strconv.Itoa(resp.StatusCode)
	}
	return out, classify(resp.StatusCode, out)
}

// classify: status_code 200/201 = accepted (200 settled/found, 201 pending/created). 202 (e.g. expire),
// 406 (duplicate order_id), 408, 429, and 5xx leave the outcome unknown or need an inquiry. 400, 401,
// 402, 404 and other 4xx are definitive. 404 is handled by the caller (inquiry vs create).
func classify(httpStatus int, r response) error {
	code, _ := strconv.Atoi(r.StatusCode)
	switch {
	case httpStatus >= 500, code >= 500:
		return &gateway.Error{Kind: gateway.Ambiguous, Code: r.StatusCode, HTTPStatus: httpStatus, Retryable: true, Message: "gateway outcome unknown"}
	case code == 200 || code == 201:
		return nil
	case code == 202 || code == 406 || code == 408 || code == 429:
		return &gateway.Error{Kind: gateway.Ambiguous, Code: r.StatusCode, HTTPStatus: httpStatus, Retryable: true, Message: "gateway outcome unknown"}
	default:
		return &gateway.Error{Kind: gateway.Rejected, Code: r.StatusCode, HTTPStatus: httpStatus, Message: safeMsg(r.Message)}
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
	if !orderIDPattern.MatchString(r.Reference) || r.Amount <= 0 || r.Amount > 1<<31-1 || r.Currency != "IDR" || r.ExpiresAt.IsZero() {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	left := r.ExpiresAt.Sub(a.cfg.Now())
	if left <= 0 {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "expiry is in the past"}
	}
	minutes := int((left + time.Minute - 1) / time.Minute) // round up: never shorter than asked
	payload := map[string]any{
		"payment_type":        "qris",
		"transaction_details": map[string]any{"order_id": r.Reference, "gross_amount": r.Amount},
		"qris":                map[string]string{"acquirer": "gopay"},
		"custom_expiry":       map[string]any{"expiry_duration": minutes, "unit": "minute"},
	}
	res, err := a.do(ctx, http.MethodPost, "/v2/charge", r.Reference, payload)
	if err != nil {
		return gateway.CreateResult{}, err
	}
	if res.TransactionID == "" || res.QRString == "" {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, Code: res.StatusCode, Message: "incomplete create response"}
	}
	return gateway.CreateResult{GatewayReference: res.TransactionID, QRPayload: res.QRString, RawStatus: res.Status, ResponseCode: res.StatusCode}, nil
}

// GetPayment asks by our order_id (the payment id), so it also resolves a create that ended Ambiguous.
// 404 then means Midtrans has no such order yet; that stays Ambiguous (the create may still be in flight).
func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	if !orderIDPattern.MatchString(r.Reference) {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid query request"}
	}
	res, err := a.do(ctx, http.MethodGet, "/v2/"+url.PathEscape(r.Reference)+"/status", "", nil)
	if err != nil {
		if ge := (*gateway.Error)(nil); errors.As(err, &ge) && ge.Code == "404" {
			return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, Code: "404", HTTPStatus: ge.HTTPStatus, Message: "order not found at midtrans"}
		}
		return gateway.QueryResult{}, err
	}
	if res.Status == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, Code: res.StatusCode, Message: "status response without status"}
	}
	out := gateway.QueryResult{RawStatus: res.Status, Amount: -1, Currency: res.Currency, ResponseCode: res.StatusCode}
	if v, err := wholeRupiah(res.GrossAmount); err == nil && v > 0 {
		out.Amount = v
	}
	// A settlement flagged by fraud screening is not money in hand: leave it unmapped.
	if res.Status == "settlement" && res.FraudStatus != "" && res.FraudStatus != "accept" {
		out.RawStatus = "fraud-" + res.FraudStatus
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", res.SettleTime, wib); err == nil && out.RawStatus == "settlement" {
		out.PaidAt = &t
	}
	return out, nil
}

func (a *Adapter) CancelPayment(context.Context, gateway.CancelRequest) error {
	return gateway.ErrNotSupported
}

// capture/authorize (cards), refund/chargeback and fraud-* stay unmapped on purpose: callers leave the
// payment unchanged.
var statusTable = map[string]gateway.Status{
	"pending":    gateway.StatusPending,
	"settlement": gateway.StatusSucceeded,
	"expire":     gateway.StatusExpired,
	"cancel":     gateway.StatusCancelled,
	"deny":       gateway.StatusFailed,
	"failure":    gateway.StatusFailed,
}

func (a *Adapter) MapStatus(raw string) (gateway.Status, bool) {
	s, ok := statusTable[raw]
	return s, ok
}

// wholeRupiah accepts "10000" and "10000.00"; any other fraction is rejected, never rounded.
func wholeRupiah(s string) (int64, error) {
	whole, frac, _ := strings.Cut(s, ".")
	if strings.Trim(frac, "0") != "" {
		return 0, errors.New("midtrans: fractional amount")
	}
	return strconv.ParseInt(whole, 10, 64)
}

var _ gateway.Adapter = (*Adapter)(nil)
