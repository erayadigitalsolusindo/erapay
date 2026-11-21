// Package dana is the adapter for DANA QRIS Acquirer MPM (SNAP, asymmetric signature). Create and inquiry only.
// See docs/integrations/dana.md; items marked BELUM JELAS there are not guessed here.
//
// Not implemented: cancel and refund (semantics unconfirmed), CPM, payouts, settlement parser. Payment status
// moves through inquiry; Finish Notify is verified (webhook.go) but must still be confirmed by inquiry.
package dana

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const (
	Code = "dana"

	SandboxURL    = "https://api.sandbox.dana.id"
	ProductionURL = "https://api.saas.dana.id"

	pathGenerate = "/v1.0/qr/qr-mpm-generate.htm"
	pathQuery    = "/rest/v1.1/debit/status"
	pathConsult  = "/v1.0/payment-gateway/consult-pay.htm"

	// Gapura Custom Checkout, QRIS only (ModeGapura): Create Order (SNAP 54) and Query Payment (serviceCode 54).
	pathGapuraCreate = "/payment-gateway/v1.0/debit/payment-host-to-host.htm"
	pathGapuraQuery  = "/payment-gateway/v1.0/debit/status.htm"
	serviceGapura    = "54"
	rcGapuraCreateOK = "2005400"

	ModeCore   = "core"   // QRIS Acquirer MPM (Generate QRIS), the default
	ModeGapura = "gapura" // Gapura Create Order with payOption NETWORK_PAY_PG_QRIS

	serviceQRISMPM = "47"

	rcGenerateOK    = "2004700"
	rcQueryOK       = "2005500"
	rcConsultOK     = "2000000"
	rcQueryNotFound = "4045501"

	// rawNotFound is the DANA status "07" (order not found); a 4045501 answer is reported as the same.
	rawNotFound = "07"

	// defaultChannelID is the CHANNEL-ID DANA's official SDKs send on every request (dana-id/dana-go, utils/snap_header.go).
	defaultChannelID = "95221"
)

var wib = time.FixedZone("WIB", 7*3600)

type Config struct {
	BaseURL       string // SandboxURL or ProductionURL
	MerchantID    string
	ClientID      string // X-PARTNER-ID
	StoreID       string // required by Generate QRIS
	ChannelID     string // CHANNEL-ID header, max 5 chars; empty = defaultChannelID
	PrivateKeyPEM string // RSA, PKCS#8 or PKCS#1. Secret.
	PublicKeyPEM  string // DANA's public key, verifies Finish Notify. Optional until the webhook is used.
	Mode          string // ModeCore (empty) or ModeGapura
	MCC           string // merchant category code, Gapura only
	CallbackURL   string // Finish Notify URL sent in Create Order, Gapura only (https)
	ReturnURL     string // PAY_RETURN URL, mandatory in Create Order though a QR payer is never redirected, Gapura only (https)
	HTTP          *http.Client
	Now           func() time.Time
}

type Adapter struct {
	cfg  Config
	priv *rsa.PrivateKey
	pub  *rsa.PublicKey // nil = webhooks cannot be verified
}

func New(cfg Config) (*Adapter, error) {
	if cfg.ChannelID == "" {
		cfg.ChannelID = defaultChannelID
	}
	if cfg.MerchantID == "" || cfg.ClientID == "" || cfg.StoreID == "" {
		return nil, errors.New("dana: merchant id, client id and store id are required")
	}
	switch cfg.Mode {
	case "":
		cfg.Mode = ModeCore
	case ModeCore:
	case ModeGapura:
		for _, v := range []string{cfg.CallbackURL, cfg.ReturnURL} {
			if u, err := url.Parse(v); err != nil || u.Host == "" || u.Scheme != "https" || len(v) > 512 {
				return nil, errors.New("dana: gapura needs https callback and return URLs (max 512)")
			}
		}
		if cfg.MCC == "" || len(cfg.MCC) > 64 {
			return nil, errors.New("dana: gapura needs an MCC")
		}
	default:
		return nil, errors.New("dana: mode must be core or gapura")
	}
	if len(cfg.ClientID) > 36 || len(cfg.ChannelID) > 5 {
		return nil, errors.New("dana: client id (max 36) or channel id (max 5) is too long")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("dana: base URL must be an absolute http(s) URL")
	}
	priv, err := parsePrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	a := &Adapter{cfg: cfg, priv: priv}
	if strings.TrimSpace(cfg.PublicKeyPEM) != "" {
		if a.pub, err = parsePublicKey(cfg.PublicKeyPEM); err != nil {
			return nil, err
		}
	}
	a.cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if a.cfg.HTTP == nil {
		// DANA expects an answer within 8s; a little more so a slow answer is not cut off first.
		a.cfg.HTTP = &http.Client{Timeout: 12 * time.Second}
	}
	if a.cfg.Now == nil {
		a.cfg.Now = time.Now
	}
	return a, nil
}

// keyDER decodes a key given as PEM, or as the bare base64 DER that DANA's dashboard copies (no BEGIN/END lines).
// Lines are trimmed: the DANA public key in their docs has a stray space before the END line.
func keyDER(s string) ([]byte, error) {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	s = strings.Join(lines, "\n")
	if strings.Contains(s, "-----BEGIN") {
		blk, _ := pem.Decode([]byte(s))
		if blk == nil {
			return nil, errors.New("not PEM")
		}
		return blk.Bytes, nil
	}
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
	if err != nil || len(der) == 0 {
		return nil, errors.New("neither PEM nor base64")
	}
	return der, nil
}

func parsePrivateKey(s string) (*rsa.PrivateKey, error) {
	der, err := keyDER(s)
	if err != nil {
		return nil, errors.New("dana: private key is " + err.Error())
	}
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, errors.New("dana: private key is not RSA")
	}
	k, err := x509.ParsePKCS1PrivateKey(der)
	if err != nil {
		return nil, errors.New("dana: private key is neither PKCS#8 nor PKCS#1")
	}
	return k, nil
}

func parsePublicKey(s string) (*rsa.PublicKey, error) {
	der, err := keyDER(s)
	if err != nil {
		return nil, errors.New("dana: public key is " + err.Error())
	}
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, errors.New("dana: public key is not PKIX")
	}
	rk, ok := k.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("dana: public key is not RSA")
	}
	return rk, nil
}

func (a *Adapter) Code() string { return Code }

func (a *Adapter) Capabilities() gateway.Capabilities {
	// One merchant id for the platform. Cancel/refund/payout/settlement are not advertised (unconfirmed).
	return gateway.Capabilities{PaymentMethods: []string{"QRIS"}, Inquiry: true, PlatformAccount: true, Hosted: a.cfg.Mode == ModeGapura}
}

// ---- references and money ----

// shortRef packs our payment UUID into DANA's 25 character partnerReferenceNo limit (QRIS): the 128 bits as
// base36 need exactly 25 digits (36^25 > 2^128). Reversible, so no mapping table is needed.
func shortRef(id string) (string, bool) {
	h := strings.ReplaceAll(id, "-", "")
	n, ok := new(big.Int).SetString(h, 16)
	if !ok || len(h) != 32 || len(id) != 36 {
		return "", false
	}
	s := n.Text(36)
	s = strings.Repeat("0", 25-len(s)) + s
	if back, ok := LongRef(s); !ok || back != id { // reject non canonical (uppercase) ids
		return "", false
	}
	return s, true
}

// LongRef reverses shortRef: DANA's partnerReferenceNo back to our payment UUID.
func LongRef(s string) (string, bool) {
	if len(s) != 25 {
		return "", false
	}
	n, ok := new(big.Int).SetString(s, 36)
	if !ok || n.Sign() < 0 || n.BitLen() > 128 {
		return "", false
	}
	h := fmt.Sprintf("%032x", n)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], true
}

func fmtMoney(v int64) string { return strconv.FormatInt(v, 10) + ".00" }

// parseMoney accepts "10000" or "10000.00" only; a real fraction is rejected, never rounded.
func parseMoney(s string) (int64, bool) {
	whole, frac, _ := strings.Cut(s, ".")
	if strings.Trim(frac, "0") != "" || len(frac) > 2 {
		return 0, false
	}
	v, err := strconv.ParseInt(whole, 10, 64)
	return v, err == nil && v >= 0
}

// ---- signing ----

// stringToSign: METHOD:path:lowercase(hex(sha256(minified body))):timestamp (authentication-asymmetric.md).
func stringToSign(method, path string, minBody []byte, ts string) string {
	sum := sha256.Sum256(minBody)
	return method + ":" + path + ":" + hex.EncodeToString(sum[:]) + ":" + ts
}

func (a *Adapter) sign(s string) (string, error) {
	h := sha256.Sum256([]byte(s))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.priv, crypto.SHA256, h[:])
	return base64.StdEncoding.EncodeToString(sig), err
}

func timestamp(t time.Time) string { return t.In(wib).Format("2006-01-02T15:04:05-07:00") }

type money struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type envelope struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
	ReferenceNo     string `json:"referenceNo"`
	AdditionalInfo  struct {
		DebugMessage string `json:"debugMessage"`
		PaymentCode  string `json:"paymentCode"`
	} `json:"additionalInfo"`
	QRContent               string      `json:"qrContent"`
	LatestTransactionStatus string      `json:"latestTransactionStatus"`
	TransAmount             *money      `json:"transAmount"`
	PaidTime                string      `json:"paidTime"`
	PaymentInfos            []PayOption `json:"paymentInfos"`
	WebRedirectURL          string      `json:"webRedirectUrl"`
}

// do sends one signed POST. The body bytes that are hashed are exactly the bytes sent. A transport failure
// after the request left is Ambiguous: DANA may have created the QR, so resolve by inquiry, never by creating
// again under a new reference.
func (a *Adapter) do(ctx context.Context, path string, payload any, okCode string) (envelope, int, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // the signed bytes must equal DANA's minified form
	if err := enc.Encode(payload); err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "encode request"}
	}
	body := bytes.TrimRight(buf.Bytes(), "\n")
	ts := timestamp(a.cfg.Now())
	sig, err := a.sign(stringToSign(http.MethodPost, path, body, ts))
	if err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "sign request"}
	}
	ext := make([]byte, 16)
	_, _ = rand.Read(ext)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Rejected, Message: "build request"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-SIGNATURE", sig)
	req.Header.Set("X-PARTNER-ID", a.cfg.ClientID)
	req.Header.Set("X-EXTERNAL-ID", hex.EncodeToString(ext)) // 32 chars, unique per request
	req.Header.Set("CHANNEL-ID", a.cfg.ChannelID)
	if a.cfg.BaseURL == SandboxURL {
		// DANA's SDK sends this in sandbox: failures then carry the reason in additionalInfo.debugMessage.
		req.Header.Set("X-Debug-Mode", "true")
	}
	resp, err := a.cfg.HTTP.Do(req)
	if err != nil {
		return envelope{}, 0, &gateway.Error{Kind: gateway.Ambiguous, Retryable: true, Message: "no response from gateway"}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env envelope
	if json.Unmarshal(raw, &env) != nil && resp.StatusCode >= 400 {
		env.ResponseMessage = "respons bukan JSON dari DANA"
	}
	return env, resp.StatusCode, classify(resp.StatusCode, env, okCode)
}

// classify follows DANA's "partner action" tables. Response codes are HTTP(3)+service(2)+case(2).
// 4xx codes are definitive rejections. 5xxxx00 is a non retry-able general error (rejected); other 5xx,
// 202 and anything unrecognized leave the outcome unknown.
func classify(status int, env envelope, okCode string) error {
	rc := env.ResponseCode
	if status >= 200 && status < 300 && rc == okCode {
		return nil
	}
	if len(rc) == 7 {
		switch {
		case rc[0] == '4':
			return &gateway.Error{Kind: gateway.Rejected, Code: rc, HTTPStatus: status, Retryable: rc[:3] == "429", Message: reason(env)}
		case rc[:3] == "500" && rc[5:] == "00":
			return &gateway.Error{Kind: gateway.Rejected, Code: rc, HTTPStatus: status, Message: reason(env)}
		}
	} else if status >= 400 && status < 500 {
		return &gateway.Error{Kind: gateway.Rejected, Code: rc, HTTPStatus: status, Message: reason(env)}
	}
	msg := "gateway outcome unknown"
	if r := reason(env); r != "" { // DANA's own words (and sandbox debugMessage) make a 5xx diagnosable
		msg += ": " + r
	}
	return &gateway.Error{Kind: gateway.Ambiguous, Code: rc, HTTPStatus: status, Retryable: true, Message: msg}
}

// reason is the sanitized text shown to operators: DANA's message plus, in sandbox, its debug reason.
func reason(env envelope) string {
	m := safeMsg(env.ResponseMessage)
	if d := safeMsg(env.AdditionalInfo.DebugMessage); d != "" {
		m += ": " + d
	}
	return safeMsg(m)
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
	if r.Method != "QRIS" {
		return gateway.CreateResult{}, gateway.ErrNotSupported
	}
	ref, ok := shortRef(r.Reference)
	if !ok || r.Amount <= 0 || r.Currency != "IDR" || r.ExpiresAt.IsZero() || !r.ExpiresAt.After(a.cfg.Now()) {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	payload := map[string]any{
		"merchantId": a.cfg.MerchantID, "storeId": a.cfg.StoreID, "partnerReferenceNo": ref,
		"amount":         money{fmtMoney(r.Amount), "IDR"},
		"validityPeriod": timestamp(r.ExpiresAt),
		// The payer is anonymous to us; envInfo only carries the mandatory platform/terminal markers.
		"additionalInfo": map[string]any{"envInfo": map[string]string{"sourcePlatform": "IPG", "orderTerminalType": "SYSTEM", "terminalType": "SYSTEM"}},
	}
	if a.cfg.Mode == ModeGapura {
		return a.createGapura(ctx, r, ref)
	}
	env, status, err := a.do(ctx, pathGenerate, payload, rcGenerateOK)
	if err != nil {
		return gateway.CreateResult{}, err
	}
	if env.QRContent == "" {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "incomplete create response"}
	}
	gref := env.ReferenceNo
	if gref == "" {
		gref = r.Reference
	}
	return gateway.CreateResult{GatewayReference: gref, QRPayload: env.QRContent, RawStatus: "01", ResponseCode: env.ResponseCode}, nil
}

func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	ref, ok := shortRef(r.Reference)
	if !ok {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid query request"}
	}
	path, svc := a.queryTarget()
	payload := map[string]string{"originalPartnerReferenceNo": ref, "serviceCode": svc, "merchantId": a.cfg.MerchantID}
	env, status, err := a.do(ctx, path, payload, rcQueryOK)
	if env.ResponseCode == rcQueryNotFound {
		return gateway.QueryResult{RawStatus: rawNotFound, Amount: -1, ResponseCode: env.ResponseCode}, nil
	}
	if err != nil {
		return gateway.QueryResult{}, err
	}
	if env.LatestTransactionStatus == "" {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, HTTPStatus: status, Message: "status response without status"}
	}
	res := gateway.QueryResult{RawStatus: env.LatestTransactionStatus, Amount: -1, ResponseCode: env.ResponseCode}
	if m := env.TransAmount; m != nil {
		res.Currency = m.Currency
		if v, ok := parseMoney(m.Value); ok {
			res.Amount = v
		}
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z07:00", env.PaidTime); err == nil {
		res.PaidAt = &t
	}
	return res, nil
}

// queryTarget is the inquiry endpoint and service code of the configured mode (same response shape in both).
func (a *Adapter) queryTarget() (path, serviceCode string) {
	if a.cfg.Mode == ModeGapura {
		return pathGapuraQuery, serviceGapura
	}
	return pathQuery, serviceQRISMPM
}

// CancelPayment: DANA's Cancel is documented after payment ("void"); whether it closes an unpaid QR is
// unconfirmed (docs/integrations/dana.md). An unpaid QR expires at validityPeriod.
func (a *Adapter) CancelPayment(context.Context, gateway.CancelRequest) error {
	return gateway.ErrNotSupported
}

// statusTable: 02 (paying, payment done but not final) stays pending until 00 or Finish Notify: DANA's docs
// call it success, but money is only applied on a final status. 05 is documented as "closed because expired".
// 07 (not found) is failed: DANA does not know the order, so no QR can be paid.
var statusTable = map[string]gateway.Status{
	"00": gateway.StatusSucceeded,
	"01": gateway.StatusPending,
	"02": gateway.StatusPending,
	"05": gateway.StatusExpired,
	"07": gateway.StatusFailed,
}

func (a *Adapter) MapStatus(raw string) (gateway.Status, bool) {
	s, ok := statusTable[raw]
	return s, ok
}

var _ gateway.Adapter = (*Adapter)(nil)
