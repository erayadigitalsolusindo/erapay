// Package doku is the DOKU SNAP QRIS MPM adapter. Everything here follows the [V]
// (verified) items of docs/integrations/doku.md: B2B token, symmetric request
// signature, qr-mpm-generate, qr-mpm-query and qr-expire. Webhook parsing,
// refund and settlement are intentionally absent until DOKU's payloads are
// confirmed ([?]/[KD] items). Credentials come only from the caller (env).
package doku

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
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const (
	Code           = "doku"
	SandboxBaseURL = "https://api-sandbox.doku.com"
	ProductionURL  = "https://api.doku.com"

	pathToken    = "/authorization/v1/access-token/b2b"
	pathGenerate = "/snap-adapter/b2b/v1.0/qr/qr-mpm-generate"
	pathQuery    = "/snap-adapter/b2b/v1.0/qr/qr-mpm-query"
	pathExpire   = "/snap-adapter/b2b/v1.0/qr/qr-expire"

	// Kirim DOKU disbursement (doku.md §6). Semua path [V] dari halaman resmi
	// developers.doku.com/payout/kirim-doku/*.md (14 Okt 2026) — termasuk
	// check-status yang pakai path qr-mpm-status (halamannya sendiri, bukan typo QRIS).
	pathInquiry  = "/snap/v1.1/emoney/bank-account-inquiry"
	pathTransfer = "/snap/v1.1/emoney/transfer-bank"
	pathCheck    = "/snap/v1.1/qr/qr-mpm-status"
	pathBalance  = "/snap/v1.1/balance-inquiry"

	serviceCodeQuery = "47" // doku.md §3 (Query)
	feeTypeFixed     = "1"  // doku.md §3: only value allowed
)

type Config struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	PrivateKey   *rsa.PrivateKey // merchant private key for the B2B token signature
	TerminalID   string          // alphanumeric 3-16
	PostalCode   string
	HTTP         *http.Client
	Now          func() time.Time

	// StatusMapConfirmed enables the status mapping table. The QRIS query status codes
	// are NOT yet confirmed for QRIS ([?] in doku.md §3); until the operator has verified
	// them in the DOKU sandbox and sets this, every status is "unmapped" and payments
	// stay unchanged instead of being guessed.
	StatusMapConfirmed bool
}

type Adapter struct {
	cfg Config
	seq atomic.Uint64

	mu      sync.Mutex
	token   string
	tokenAt time.Time // expiry
}

func New(cfg Config) (*Adapter, error) {
	switch {
	case cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.PrivateKey == nil:
		return nil, errors.New("doku: client id, client secret and private key are required")
	case !terminalRe.MatchString(cfg.TerminalID):
		return nil, errors.New("doku: terminal id must be 3-16 alphanumeric characters")
	case cfg.PostalCode == "":
		return nil, errors.New("doku: postal code is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Adapter{cfg: cfg}, nil
}

var terminalRe = regexp.MustCompile(`^[A-Za-z0-9]{3,16}$`)

// ParsePrivateKey accepts PKCS#8 or PKCS#1 PEM.
func ParsePrivateKey(p []byte) (*rsa.PrivateKey, error) {
	blk, _ := pem.Decode(p)
	if blk == nil {
		return nil, errors.New("doku: private key is not PEM")
	}
	if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, errors.New("doku: private key is not RSA")
	}
	return x509.ParsePKCS1PrivateKey(blk.Bytes)
}

func (a *Adapter) Code() string { return Code }

func (a *Adapter) Capabilities() gateway.Capabilities {
	return gateway.Capabilities{PaymentMethods: []string{"QRIS"}, Inquiry: true, Cancel: true, Disbursement: true}
	// Refund, settlement report: unconfirmed ([?]) -> not advertised.
}

// ---- signing (doku.md §2) ----

func ts(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05Z") }

// tokenSignature = base64(SHA256withRSA(privateKey, clientID|timestamp)).
func (a *Adapter) tokenSignature(timestamp string) (string, error) {
	d := sha256.Sum256([]byte(a.cfg.ClientID + "|" + timestamp))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.cfg.PrivateKey, crypto.SHA256, d[:])
	return base64.StdEncoding.EncodeToString(sig), err
}

// RequestSignature = base64(HMAC-SHA512(secret, METHOD:path:token:lowerhex(sha256(body)):timestamp)).
// body must be the exact bytes sent. Exposed for tests of the algorithm.
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
	stamp := ts(now)
	sig, err := a.tokenSignature(stamp)
	if err != nil {
		return "", &gateway.Error{Kind: gateway.Rejected, Message: "token signing failed"}
	}
	body := []byte(`{"grantType":"client_credentials"}`)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+pathToken, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CLIENT-KEY", a.cfg.ClientID)
	req.Header.Set("X-TIMESTAMP", stamp)
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
		// DOKU responseMessage names the reason (unknown client, bad signature); it carries no secret.
		msg := "could not obtain access token"
		if m := strings.TrimSpace(out.ResponseMsg); m != "" {
			if len(m) > 200 {
				m = m[:200]
			}
			msg += ": " + m
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

type envelope struct {
	ResponseCode            string `json:"responseCode"`
	ResponseMessage         string `json:"responseMessage"`
	ReferenceNo             string `json:"referenceNo"`
	QRContent               string `json:"qrContent"`
	LatestTransactionStatus string `json:"latestTransactionStatus"`
	TransactionStatusDesc   string `json:"transactionStatusDesc"`
	PaidTime                string `json:"paidTime"`
	Amount                  *struct {
		Value    string `json:"value"`
		Currency string `json:"currency"`
	} `json:"amount"`

	// Kirim DOKU disbursement fields (doku.md §6).
	PartnerReferenceNo       string  `json:"partnerReferenceNo"`
	OriginalReferenceNo      string  `json:"originalReferenceNo"`
	SessionID                string  `json:"sessionId"`
	TransactionDate          string  `json:"transactionDate"`
	ReferenceNumber          string  `json:"referenceNumber"`
	BeneficiaryAccountName   string  `json:"beneficiaryAccountName"`
	BeneficiaryBankCode      string  `json:"beneficiaryBankCode"`
	BeneficiaryBankName      string  `json:"beneficiaryBankName"`
	BeneficiaryBankShortName string  `json:"beneficiaryBankShortName"`
	AdditionalInfo           *kdInfo `json:"additionalInfo"`

	// Kirim DOKU balance inquiry (doku.md §6.3).
	AccountNo    string `json:"accountNo"`
	Name         string `json:"name"`
	AccountInfos *struct {
		HoldAmount       *amountField `json:"holdAmount"`
		AvailableBalance *amountField `json:"avaliableBalance"` // [V] typo DOKU "avaliableBalance"
	} `json:"accountInfos"`
}

// amountField adalah DTO {value, currency} respons amount DOKU.
type amountField struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// kdInfo is the additionalInfo object of the Kirim DOKU responses.
type kdInfo struct {
	SessionID         string `json:"sessionId"`
	Amount            string `json:"amount"`
	FeeAmount         string `json:"feeAmount"`
	FeeCurrency       string `json:"feeCurrency"`
	BeneficiaryAmount string `json:"beneficiaryAmount"`
	CashInTime        string `json:"cashInTime"`
	Remark            string `json:"remark"`
}

func (a *Adapter) externalID() string {
	// numeric string, unique per day (doku.md §2)
	return strconv.FormatUint(uint64(a.cfg.Now().UnixMicro())*1000+a.seq.Add(1)%1000, 10)
}

// call performs one signed POST and classifies the outcome (see gateway.Kind).
func (a *Adapter) call(ctx context.Context, path string, payload any) (envelope, int, error) {
	body, err := json.Marshal(payload) // compact, and these exact bytes are hashed AND sent
	if err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "encode request"}
	}
	for attempt := 0; ; attempt++ {
		tok, err := a.accessToken(ctx, attempt > 0)
		if err != nil {
			return envelope{}, 0, err
		}
		stamp := ts(a.cfg.Now())
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("X-PARTNER-ID", a.cfg.ClientID)
		req.Header.Set("X-EXTERNAL-ID", a.externalID())
		req.Header.Set("CHANNEL-ID", "H2H")
		req.Header.Set("X-TIMESTAMP", stamp)
		req.Header.Set("X-SIGNATURE", RequestSignature(a.cfg.ClientSecret, http.MethodPost, path, tok, body, stamp))

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

// classify implements the ambiguity rules: 5xx, 202-style pending and 409 (duplicate
// request ID: original outcome unknown) are AMBIGUOUS; other non-2xx are definitive rejections.
func classify(status int, env envelope) error {
	code := env.ResponseCode
	switch {
	case status == http.StatusOK && strings.HasPrefix(code, "200"):
		return nil
	case status == http.StatusAccepted, strings.HasPrefix(code, "202"), status >= 500, status == http.StatusConflict, status == http.StatusRequestTimeout:
		return &gateway.Error{Kind: gateway.Ambiguous, Code: code, HTTPStatus: status, Retryable: true, Message: "gateway outcome unknown"}
	default:
		return &gateway.Error{Kind: gateway.Rejected, Code: code, HTTPStatus: status,
			Retryable: status == http.StatusTooManyRequests, Message: safeMsg(env.ResponseMessage)}
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

// ---- amount helpers: DOKU wants "^\d{1,16}\.00$" ----

var amountRe = regexp.MustCompile(`^(\d{1,16})\.00$`)

func fmtAmount(v int64) string { return strconv.FormatInt(v, 10) + ".00" }

func parseAmount(s string) int64 {
	m := amountRe.FindStringSubmatch(s)
	if m == nil {
		return -1
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return -1
	}
	return n
}

// ---- operations ----

type amountDTO struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

func (a *Adapter) CreatePayment(ctx context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	if r.Method != "QRIS" {
		return gateway.CreateResult{}, gateway.ErrNotSupported
	}
	if r.Reference == "" || len(r.Reference) > 64 || r.AccountRef == "" || r.Amount <= 0 || r.Currency != "IDR" || r.ExpiresAt.IsZero() {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	// validityPeriod defaults to 30 days when omitted, so it is ALWAYS sent (doku.md §3).
	valid := r.ExpiresAt.In(time.FixedZone("WIB", 7*3600)).Format("2006-01-02T15:04:05-07:00")
	payload := map[string]any{
		"partnerReferenceNo": r.Reference,
		"amount":             amountDTO{fmtAmount(r.Amount), r.Currency},
		"merchantId":         r.AccountRef,
		"terminalId":         a.cfg.TerminalID,
		"validityPeriod":     valid,
		"additionalInfo":     map[string]string{"postalCode": a.cfg.PostalCode, "feeType": feeTypeFixed},
	}
	env, _, err := a.call(ctx, pathGenerate, payload)
	if err != nil {
		return gateway.CreateResult{ResponseCode: env.ResponseCode}, err
	}
	if env.ReferenceNo == "" || env.QRContent == "" {
		// A "success" without the data we need: we cannot tell what DOKU did.
		return gateway.CreateResult{ResponseCode: env.ResponseCode},
			&gateway.Error{Kind: gateway.Ambiguous, Code: env.ResponseCode, HTTPStatus: 200, Message: "incomplete generate response"}
	}
	return gateway.CreateResult{GatewayReference: env.ReferenceNo, QRPayload: env.QRContent, ResponseCode: env.ResponseCode}, nil
}

func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	if r.Reference == "" || r.GatewayReference == "" || r.AccountRef == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid query request"}
	}
	payload := map[string]string{
		"originalReferenceNo":        r.GatewayReference,
		"originalPartnerReferenceNo": r.Reference,
		"serviceCode":                serviceCodeQuery,
		"merchantId":                 r.AccountRef,
	}
	env, _, err := a.call(ctx, pathQuery, payload)
	if err != nil {
		return gateway.QueryResult{ResponseCode: env.ResponseCode}, err
	}
	res := gateway.QueryResult{RawStatus: env.LatestTransactionStatus, Amount: -1, ResponseCode: env.ResponseCode}
	if env.Amount != nil {
		res.Amount, res.Currency = parseAmount(env.Amount.Value), env.Amount.Currency
	}
	if t, err := time.Parse(time.RFC3339, env.PaidTime); err == nil {
		res.PaidAt = &t
	}
	return res, nil
}

func (a *Adapter) CancelPayment(ctx context.Context, r gateway.CancelRequest) error {
	if r.Reference == "" || r.GatewayReference == "" || r.AccountRef == "" {
		return &gateway.Error{Kind: gateway.Rejected, Message: "invalid cancel request"}
	}
	_, _, err := a.call(ctx, pathExpire, map[string]string{
		"partnerReferenceNo": r.Reference, "referenceNo": r.GatewayReference, "merchantId": r.AccountRef})
	return err
}

// statusTable holds the codes documented for DOKU's other SNAP services (doku.md §3).
// They are UNPROVEN for QRIS, so the table is used only when Config.StatusMapConfirmed is set.
var statusTable = map[string]gateway.Status{
	"00": gateway.StatusSucceeded,
	"01": gateway.StatusPending, // initiated
	"02": gateway.StatusPending, // paying
	"03": gateway.StatusPending,
	"05": gateway.StatusCancelled,
	"06": gateway.StatusFailed,
	// "04" refunded and "07" not found are deliberately unmapped: a refund is a separate
	// state machine (PRD §0 #9) and "not found" does not prove the payment failed.
}

func (a *Adapter) MapStatus(raw string) (gateway.Status, bool) {
	if !a.cfg.StatusMapConfirmed {
		return "", false
	}
	s, ok := statusTable[raw]
	return s, ok
}

// ---- Kirim DOKU disbursement (doku.md §6) ----

// MapTransferStatus maps the documented status codes of the check-status response:
// 00 Success, 03 Pending, 06 Failed, 04 Refunded (own state machine: payout must not
// change on it). Anything else is unmapped and callers leave the payout unchanged.
func (a *Adapter) MapTransferStatus(raw string) (gateway.TransferStatus, bool) {
	switch raw {
	case "00":
		return gateway.TransferSucceeded, true
	case "03":
		return gateway.TransferPending, true
	case "06":
		return gateway.TransferFailed, true
	case "04":
		return gateway.TransferRefunded, true
	}
	return "", false
}

// BalanceInquiry memeriksa saldo Kirim DOKU pengirim (doku.md §6.3). DOKU
// menyuruh mengecek saldo sebelum setiap transfer untuk menghindari kegagalan
// saldo kurang. Bukan bagian gateway.Disburser — executor EraPay tidak
// menggate-kan transfer pada saldo (biaya bisa phone-call ops lewat cmd/kdtest).
func (a *Adapter) BalanceInquiry(ctx context.Context, accountNo string) (BalanceResult, error) {
	if accountNo == "" || len(accountNo) > 16 {
		return BalanceResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "accountNo is required (max 16 chars)"}
	}
	payload := map[string]any{
		"partnerReferenceNo": "bal-" + a.externalID(),
		"accountNo":          accountNo,
	}
	env, _, err := a.call(ctx, pathBalance, payload)
	if err != nil {
		return BalanceResult{ResponseCode: env.ResponseCode}, err
	}
	res := BalanceResult{
		ResponseCode:     env.ResponseCode,
		AccountNo:        env.AccountNo,
		Name:             env.Name,
		AvailableBalance: -1,
		HoldAmount:       -1,
	}
	if env.AccountInfos != nil {
		if env.AccountInfos.AvailableBalance != nil {
			res.AvailableBalance = parseAmount(env.AccountInfos.AvailableBalance.Value)
		}
		if env.AccountInfos.HoldAmount != nil {
			res.HoldAmount = parseAmount(env.AccountInfos.HoldAmount.Value)
		}
	}
	if res.AvailableBalance < 0 {
		// "Sukses" tanpa data yang dibutuhkan: tidak tahu apa yang DOKU lakukan.
		return res, &gateway.Error{Kind: gateway.Ambiguous, Code: env.ResponseCode, HTTPStatus: 200,
			Message: "balance response without availableBalance"}
	}
	return res, nil
}

// BalanceResult hasil BalanceInquiry; AvailableBalance/HoldAmount = -1 bila
// tidak dilaporkan gateway.
type BalanceResult struct {
	AvailableBalance int64  `json:"available_balance"`
	HoldAmount       int64  `json:"hold_amount"`
	AccountNo        string `json:"account_no"`
	Name             string `json:"name"`
	ResponseCode     string `json:"response_code"`
}

// InquiryBankAccount is the documented account verification: it must succeed before a
// transfer can be sent, and it is partner-paid per success (doku.md §6).
func (a *Adapter) InquiryBankAccount(ctx context.Context, r gateway.BankInquiryRequest) (gateway.BankInquiryResult, error) {
	if r.CustomerNumber == "" || len(r.CustomerNumber) > 32 || r.AccountNo == "" || len(r.AccountNo) > 32 ||
		r.BankCode == "" || r.AccountName == "" || len(r.AccountName) > 64 ||
		r.SenderCountry == "" || len(r.SenderCountry) != 2 ||
		r.Amount <= 0 || r.Currency != "IDR" {
		return gateway.BankInquiryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid bank account inquiry request"}
	}
	add := map[string]string{
		"beneficiaryBankCode":    r.BankCode,
		"beneficiaryAccountName": r.AccountName,
		"senderCountryCode":      r.SenderCountry,
	}
	if r.ChannelCode != "" {
		add["channelCode"] = r.ChannelCode
	}
	payload := map[string]any{
		"customerNumber":           r.CustomerNumber,
		"beneficiaryAccountNumber": r.AccountNo,
		"beneficiaryBankCode":      r.BankCode,
		"amount":                   amountDTO{fmtAmount(r.Amount), r.Currency},
		"additionalInfo":           add,
	}
	if r.Reference != "" {
		payload["partnerReferenceNo"] = r.Reference
	}
	env, _, err := a.call(ctx, pathInquiry, payload)
	if err != nil {
		return gateway.BankInquiryResult{ResponseCode: env.ResponseCode}, err
	}
	res := gateway.BankInquiryResult{
		SessionID:     env.SessionID,
		AccountName:   env.BeneficiaryAccountName,
		BankName:      env.BeneficiaryBankName,
		BankShortName: env.BeneficiaryBankShortName,
		FeeAmount:     -1,
		ResponseCode:  env.ResponseCode,
	}
	if env.AdditionalInfo != nil {
		if v := parseAmount(env.AdditionalInfo.FeeAmount); v >= 0 {
			res.FeeAmount = v
		}
	}
	if res.SessionID == "" {
		// A "success" without the data we need: we cannot tell what DOKU did.
		return res, &gateway.Error{Kind: gateway.Ambiguous, Code: env.ResponseCode, HTTPStatus: 200, Message: "inquiry response without sessionId"}
	}
	return res, nil
}

// TransferBank sends the funds. r.Reference is partnerReferenceNo and must be unique per
// attempt; a duplicate (409) is ambiguous, never re-sent blindly (PRD §0 #3).
func (a *Adapter) TransferBank(ctx context.Context, r gateway.TransferRequest) (gateway.TransferResult, error) {
	if r.Reference == "" || len(r.Reference) > 64 || r.CustomerNumber == "" || len(r.CustomerNumber) > 32 ||
		r.SessionID == "" || len(r.SessionID) > 25 || r.AccountNo == "" || len(r.AccountNo) > 32 || r.BankCode == "" ||
		r.BeneficiaryAccountName == "" || r.BeneficiaryFirstName == "" || len(r.BeneficiaryFirstName) > 64 ||
		r.BeneficiaryLastName == "" || len(r.BeneficiaryLastName) > 64 || r.BeneficiaryPhone == "" || len(r.BeneficiaryPhone) > 20 ||
		r.SenderCountry == "" || len(r.SenderCountry) != 2 || r.SenderFirstName == "" || len(r.SenderFirstName) > 64 ||
		r.SenderLastName == "" || len(r.SenderLastName) > 64 || r.SenderPersonalID == "" || len(r.SenderPersonalID) > 32 ||
		r.SenderIDType == "" || len(r.SenderIDType) > 16 ||
		r.Amount <= 0 || r.Currency != "IDR" {
		return gateway.TransferResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid transfer request"}
	}
	add := map[string]string{
		"beneficiaryFirstName":   r.BeneficiaryFirstName,
		"beneficiaryLastName":    r.BeneficiaryLastName,
		"beneficiaryPhoneNumber": r.BeneficiaryPhone,
		"beneficiaryAccountName": r.BeneficiaryAccountName,
		"senderCountryCode":      r.SenderCountry,
		"senderFirstName":        r.SenderFirstName,
		"senderLastName":         r.SenderLastName,
		"senderPersonalId":       r.SenderPersonalID,
		"senderPersonalIdType":   r.SenderIDType,
	}
	if r.BankName != "" {
		add["beneficiaryBankName"] = r.BankName
	}
	if r.ChannelCode != "" {
		add["channelCode"] = r.ChannelCode
	}
	if r.Remark != "" {
		add["remark"] = r.Remark
	}
	payload := map[string]any{
		"partnerReferenceNo":       r.Reference,
		"customerNumber":           r.CustomerNumber,
		"beneficiaryAccountNumber": r.AccountNo,
		"beneficiaryBankCode":      r.BankCode,
		"amount":                   amountDTO{fmtAmount(r.Amount), r.Currency},
		"sessionId":                r.SessionID,
		"additionalInfo":           add,
	}
	env, _, err := a.call(ctx, pathTransfer, payload)
	if err != nil {
		return gateway.TransferResult{ResponseCode: env.ResponseCode}, err
	}
	res := gateway.TransferResult{GatewayReference: env.ReferenceNo, ResponseCode: env.ResponseCode}
	if t, err := time.Parse(time.RFC3339, env.TransactionDate); err == nil {
		res.TransactionDate = &t
	}
	if res.GatewayReference == "" {
		// A "success" without the data we need: we cannot tell what DOKU did.
		return res, &gateway.Error{Kind: gateway.Ambiguous, Code: env.ResponseCode, HTTPStatus: 200, Message: "transfer response without referenceNo"}
	}
	return res, nil
}

// TransferStatus checks the current outcome of a transfer. It is the only safe action
// after an ambiguous or pending transfer (doku.md §6; PRD §0 #3).
func (a *Adapter) TransferStatus(ctx context.Context, r gateway.TransferStatusRequest) (gateway.TransferStatusResult, error) {
	if (r.OriginalPartnerReference == "" && r.OriginalReference == "") || r.AccountNo == "" || r.BankCode == "" {
		return gateway.TransferStatusResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid transfer status request"}
	}
	payload := map[string]string{
		"originalPartnerReferenceNo": r.OriginalPartnerReference,
		"beneficiaryAccountNumber":   r.AccountNo,
		"beneficiaryBankCode":        r.BankCode,
	}
	if r.OriginalReference != "" {
		payload["originalReferenceNo"] = r.OriginalReference
	}
	if r.ServiceCode != "" {
		payload["serviceCode"] = r.ServiceCode
	}
	env, _, err := a.call(ctx, pathCheck, payload)
	if err != nil {
		return gateway.TransferStatusResult{ResponseCode: env.ResponseCode}, err
	}
	res := gateway.TransferStatusResult{RawStatus: env.LatestTransactionStatus, Amount: -1, ResponseCode: env.ResponseCode}
	if env.Amount != nil {
		res.Amount = parseAmount(env.Amount.Value)
	}
	if t, err := time.Parse(time.RFC3339, env.TransactionDate); err == nil {
		res.TransactionDate = &t
	}
	if res.RawStatus == "" {
		return res, &gateway.Error{Kind: gateway.Ambiguous, Code: env.ResponseCode, HTTPStatus: 200, Message: "status response without latestTransactionStatus"}
	}
	return res, nil
}

var _ gateway.Adapter = (*Adapter)(nil)
