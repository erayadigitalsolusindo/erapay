// Package bri is the Bank BRI adapter (BRIAPI SNAP-BI) for QRIS MPM dinamis. It follows
// docs/integrations/bri.md: asymmetric B2B token, symmetric (HMAC-SHA512) request signature,
// qr-mpm-generate and qr-mpm-query. QRIS only receives money: no cancel, refund or payout is
// advertised here (payouts are a separate transfer API, see bri.md §6). Credentials come only
// from the caller.
package bri

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const (
	Code           = "bri"
	SandboxBaseURL = "https://sandbox.partner.api.bri.co.id"

	pathToken    = "/snap/v1.0/access-token/b2b"
	pathGenerate = "/snap/v1.1/qr/qr-mpm-generate"
	pathQuery    = "/snap/v1.1/qr/qr-mpm-query"

	methodQRIS = "QRIS"

	defaultQueryServiceCode = "47" // bri.md §4; confirm with BRI
)

// wib is the fixed UTC+7 zone BRI's examples use for X-TIMESTAMP.
var wib = time.FixedZone("WIB", 7*60*60)

type Config struct {
	BaseURL      string
	ClientID     string          // X-CLIENT-KEY for the token call
	ClientSecret string          // HMAC key for business requests
	PrivateKey   *rsa.PrivateKey // pair of the public key registered in the BRI devportal
	PartnerID    string          // X-PARTNER-ID (max 36)
	ChannelID    string          // CHANNEL-ID (max 5)
	MerchantID   string          // QRIS merchantId (max 64)
	TerminalID   string          // QRIS terminalId (max 16)
	// QueryServiceCode is the serviceCode sent with qr-mpm-query; "" = default.
	QueryServiceCode string
	HTTP             *http.Client
	Now              func() time.Time
}

type Adapter struct {
	cfg Config

	mu      sync.Mutex
	token   string
	tokenAt time.Time // refresh deadline
}

var (
	partnerRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,36}$`)
	channelRe  = regexp.MustCompile(`^[A-Za-z0-9]{1,5}$`)
	merchantRe = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)
	terminalRe = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)
)

func New(cfg Config) (*Adapter, error) {
	switch {
	case cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.PrivateKey == nil:
		return nil, errors.New("bri: client id, client secret and private key are required")
	case !partnerRe.MatchString(cfg.PartnerID):
		return nil, errors.New("bri: partner id must be 1-36 characters")
	case !channelRe.MatchString(cfg.ChannelID):
		return nil, errors.New("bri: channel id must be 1-5 alphanumeric characters")
	case !merchantRe.MatchString(cfg.MerchantID):
		return nil, errors.New("bri: merchant id must be 1-64 alphanumeric characters")
	case !terminalRe.MatchString(cfg.TerminalID):
		return nil, errors.New("bri: terminal id must be 1-16 alphanumeric characters")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.QueryServiceCode == "" {
		cfg.QueryServiceCode = defaultQueryServiceCode
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Adapter{cfg: cfg}, nil
}

// ParsePrivateKey accepts PKCS#8 or PKCS#1 PEM.
func ParsePrivateKey(p []byte) (*rsa.PrivateKey, error) {
	blk, _ := pem.Decode(p)
	if blk == nil {
		return nil, errors.New("bri: private key is not PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, errors.New("bri: private key is not RSA")
	}
	return x509.ParsePKCS1PrivateKey(blk.Bytes)
}

func (a *Adapter) Code() string { return Code }

func (a *Adapter) Capabilities() gateway.Capabilities {
	// BRI is contracted once for the whole platform (the aggregator's own QRIS merchant), so no
	// per-merchant account is needed. Cancel and refund are not offered for QRIS.
	return gateway.Capabilities{PaymentMethods: []string{methodQRIS}, Inquiry: true, PlatformAccount: true}
}

// ---- signing (bri.md §3) ----

func stamp(t time.Time) string { return t.In(wib).Format("2006-01-02T15:04:05.000-07:00") }

// tokenSignature = base64(SHA256withRSA(privateKey, clientID|timestamp)).
func (a *Adapter) tokenSignature(timestamp string) (string, error) {
	d := sha256.Sum256([]byte(a.cfg.ClientID + "|" + timestamp))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.cfg.PrivateKey, crypto.SHA256, d[:])
	return base64.StdEncoding.EncodeToString(sig), err
}

// RequestSignature = base64(HMAC-SHA512(secret, METHOD:path:token:lowerhex(sha256(body)):timestamp)).
// body must be the exact bytes sent. The base64 output encoding follows the SNAP-BI standard; the
// BRI page only says "HMAC_SHA512", so it is confirmed against the sandbox (bri.md §7).
func RequestSignature(secret, method, path, accessToken string, body []byte, timestamp string) string {
	h := sha256.Sum256(body)
	s := method + ":" + path + ":" + accessToken + ":" + hex.EncodeToString(h[:]) + ":" + timestamp
	m := hmac.New(sha512.New, []byte(secret))
	m.Write([]byte(s))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// ---- token (cached; refreshed before the 15 minute expiry) ----

type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	n, err := strconv.Atoi(strings.Trim(string(b), `"`))
	*f = flexInt(n)
	return err
}

func (a *Adapter) accessToken(ctx context.Context, force bool) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.cfg.Now()
	if !force && a.token != "" && now.Before(a.tokenAt) {
		return a.token, nil
	}
	a.token = ""
	ts := stamp(now)
	sig, err := a.tokenSignature(ts)
	if err != nil {
		return "", &gateway.Error{Kind: gateway.Rejected, Message: "token signing failed"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+pathToken,
		strings.NewReader(`{"grantType":"client_credentials"}`))
	if err != nil {
		return "", &gateway.Error{Kind: gateway.Rejected, Message: "build token request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CLIENT-KEY", a.cfg.ClientID)
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-SIGNATURE", sig)
	resp, err := a.cfg.HTTP.Do(req)
	if err != nil {
		return "", &gateway.Error{Kind: gateway.Ambiguous, Retryable: true, Message: "token request failed"}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out struct {
		AccessToken  string  `json:"accessToken"`
		ExpiresIn    flexInt `json:"expiresIn"`
		ResponseCode string  `json:"responseCode"`
		ResponseMsg  string  `json:"responseMessage"`
	}
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode != http.StatusOK || out.AccessToken == "" {
		// No payment is attached to a token call, so a failure is safe to report as a plain error.
		msg := "could not obtain access token"
		if m := strings.TrimSpace(out.ResponseMsg); m != "" {
			msg += ": " + safeMsg(m)
		}
		return "", &gateway.Error{Kind: gateway.Rejected, Code: out.ResponseCode, HTTPStatus: resp.StatusCode,
			Retryable: resp.StatusCode >= 500, Message: msg}
	}
	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 900 * time.Second
	}
	a.token = out.AccessToken
	a.tokenAt = now.Add(ttl - 60*time.Second) // refresh a minute early
	return a.token, nil
}

// ---- transport ----

type amountDTO struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type envelope struct {
	ResponseCode            string     `json:"responseCode"`
	ResponseMessage         string     `json:"responseMessage"`
	PartnerReferenceNo      string     `json:"partnerReferenceNo"`
	ReferenceNo             string     `json:"referenceNo"`
	OriginalReferenceNo     string     `json:"originalReferenceNo"`
	QRContent               string     `json:"qrContent"`
	LatestTransactionStatus string     `json:"latestTransactionStatus"`
	PaidTime                string     `json:"paidTime"`
	Amount                  *amountDTO `json:"amount"`
}

// externalID: numeric, unique per request (X-EXTERNAL-ID, max 36).
func externalID(now time.Time) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return strconv.FormatInt(now.UnixMilli(), 10) + fmt.Sprintf("%010d", binary.BigEndian.Uint32(b[:]))
}

// call performs one signed POST and classifies the outcome (see gateway.Kind).
func (a *Adapter) call(ctx context.Context, path string, payload any) (envelope, int, error) {
	body, err := json.Marshal(payload) // compact; these exact bytes are hashed AND sent
	if err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "encode request"}
	}
	for attempt := 0; ; attempt++ {
		tok, err := a.accessToken(ctx, attempt > 0)
		if err != nil {
			return envelope{}, 0, err
		}
		now := a.cfg.Now()
		ts := stamp(now)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+path, bytes.NewReader(body))
		if err != nil {
			return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "build request"}
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("X-PARTNER-ID", a.cfg.PartnerID)
		req.Header.Set("X-EXTERNAL-ID", externalID(now))
		req.Header.Set("CHANNEL-ID", a.cfg.ChannelID)
		req.Header.Set("X-TIMESTAMP", ts)
		req.Header.Set("X-SIGNATURE", RequestSignature(a.cfg.ClientSecret, http.MethodPost, path, tok, body, ts))

		resp, err := a.cfg.HTTP.Do(req)
		if err != nil {
			// Timeout / connection drop after sending: the request MAY have been processed.
			return envelope{}, 0, &gateway.Error{Kind: gateway.Ambiguous, Retryable: true, Message: "no response from gateway"}
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		var env envelope
		_ = json.Unmarshal(raw, &env)

		// 401 = rejected before processing (e.g. expired token): refresh once and resend.
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			continue
		}
		return env, resp.StatusCode, classify(resp.StatusCode, env)
	}
}

// classify: 200 with a 200xxxx code is success. The documented definitive failures (400, 401, 403,
// 404) are Rejected. Everything else (202, 409, 5xx, timeouts and any code BRI does not list)
// counts as pending per BRI and is Ambiguous: resolve by inquiry, never re-create blindly.
func classify(status int, env envelope) error {
	code := env.ResponseCode
	switch {
	case status == http.StatusOK && strings.HasPrefix(code, "200"):
		return nil
	case status == http.StatusBadRequest, status == http.StatusUnauthorized,
		status == http.StatusForbidden, status == http.StatusNotFound:
		return &gateway.Error{Kind: gateway.Rejected, Code: code, HTTPStatus: status, Message: safeMsg(env.ResponseMessage)}
	default:
		return &gateway.Error{Kind: gateway.Ambiguous, Code: code, HTTPStatus: status, Retryable: true, Message: "gateway outcome unknown"}
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

// ---- amounts ----

func formatAmount(n int64) string { return fmt.Sprintf("%d.00", n) }

// parseAmount reads "2000.00" into whole rupiah; -1 when it is not a usable integer amount.
func parseAmount(v *amountDTO) int64 {
	if v == nil || v.Currency != "IDR" {
		return -1
	}
	whole, frac, _ := strings.Cut(v.Value, ".")
	if strings.Trim(frac, "0") != "" {
		return -1
	}
	n, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || n < 0 {
		return -1
	}
	return n
}

// partnerRef derives a 13 digit numeric partnerReferenceNo from our payment id. BRI documents the
// field inconsistently (6 vs 13 digits) and only echoes the last 6 in notifications, so the
// payment is matched by BRI's referenceNo, never by this value (bri.md §4).
func partnerRef(reference string) string {
	h := sha256.Sum256([]byte(reference))
	n := new(big.Int).SetBytes(h[:])
	n.Mod(n, big.NewInt(10_000_000_000_000))
	return fmt.Sprintf("%013d", n)
}

// ---- operations ----

func (a *Adapter) CreatePayment(ctx context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	if r.Method != methodQRIS {
		return gateway.CreateResult{}, gateway.ErrNotSupported
	}
	if r.Reference == "" || len(r.Reference) > 64 || r.Amount <= 0 || r.Currency != "IDR" {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	payload := map[string]any{
		"partnerReferenceNo": partnerRef(r.Reference),
		"amount":             amountDTO{Value: formatAmount(r.Amount), Currency: r.Currency},
		"merchantId":         a.cfg.MerchantID,
		"terminalId":         a.cfg.TerminalID,
	}
	env, status, err := a.call(ctx, pathGenerate, payload)
	if err != nil {
		return gateway.CreateResult{}, err
	}
	if env.ReferenceNo == "" || env.QRContent == "" {
		// A "success" without the data we need: we cannot tell what BRI did.
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "incomplete create response"}
	}
	return gateway.CreateResult{GatewayReference: env.ReferenceNo, QRPayload: env.QRContent,
		RawStatus: "01", ResponseCode: env.ResponseCode}, nil
}

// GetPayment asks BRI by its own referenceNo. BRI cannot query by our reference, so without
// GatewayReference (a create whose outcome is unknown) the status cannot be resolved here.
func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	if r.GatewayReference == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, Message: "no BRI reference to query"}
	}
	payload := map[string]any{
		"originalReferenceNo": r.GatewayReference,
		"serviceCode":         a.cfg.QueryServiceCode,
		"additionalInfo":      map[string]string{"terminalId": a.cfg.TerminalID},
	}
	env, status, err := a.call(ctx, pathQuery, payload)
	if err != nil {
		return gateway.QueryResult{}, err
	}
	if env.LatestTransactionStatus == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "status response without status"}
	}
	res := gateway.QueryResult{RawStatus: env.LatestTransactionStatus, Amount: parseAmount(env.Amount), ResponseCode: env.ResponseCode}
	if env.Amount != nil {
		res.Currency = env.Amount.Currency
	}
	if t, err := time.Parse(time.RFC3339, env.PaidTime); err == nil {
		res.PaidAt = &t
	}
	return res, nil
}

// CancelPayment: BRI documents no QRIS cancel; an unpaid QR simply expires.
func (a *Adapter) CancelPayment(context.Context, gateway.CancelRequest) error {
	return gateway.ErrNotSupported
}

// statusTable maps latestTransactionStatus (bri.md §4). 04 Refunded is its own state machine and
// 07 Not found does not prove the payment failed, so both stay unmapped: callers leave the
// payment unchanged.
var statusTable = map[string]gateway.Status{
	"00": gateway.StatusSucceeded,
	"01": gateway.StatusPending,
	"02": gateway.StatusPending,
	"03": gateway.StatusPending,
	"05": gateway.StatusCancelled,
	"06": gateway.StatusFailed,
}

func (a *Adapter) MapStatus(raw string) (gateway.Status, bool) {
	s, ok := statusTable[raw]
	return s, ok
}

var (
	_ gateway.Adapter          = (*Adapter)(nil)
	_ gateway.ConnectionTester = (*Adapter)(nil)
)

// TestConnection implements gateway.ConnectionTester. Fetching a token proves the client id, the
// registered public key and the base URL without creating a QR. BRI has no cancel, so deep adds
// nothing: a QR created for testing could not be withdrawn.
func (a *Adapter) TestConnection(ctx context.Context, _ string, _ bool) []gateway.ConnectionStep {
	_, err := a.accessToken(ctx, true)
	st := gateway.ConnectionStep{Name: "credentials", OK: err == nil}
	if err != nil {
		var ge *gateway.Error
		if errors.As(err, &ge) {
			st.Code, st.Message = ge.Code, ge.Message
		} else {
			st.Message = "unexpected error"
		}
	}
	return []gateway.ConnectionStep{st}
}
