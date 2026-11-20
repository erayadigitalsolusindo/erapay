package dana

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

// AckBody is the reply DANA expects for a processed Finish Notify (2005600). Answer 5005601 to make DANA retry.
const AckBody = `{"responseCode":"2005600","responseMessage":"Successful"}`

const pathNotify = "/v1.0/debit/notify"

// WebhookAck makes the webhook handler answer with DANA's expected body instead of the generic one.
func (a *Adapter) WebhookAck() []byte { return []byte(AckBody) }

var _ gateway.WebhookAcker = (*Adapter)(nil)

type notify struct {
	OriginalPartnerReferenceNo string `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string `json:"originalReferenceNo"`
	MerchantID                 string `json:"merchantId"`
	Amount                     money  `json:"amount"`
	LatestTransactionStatus    string `json:"latestTransactionStatus"`
}

// VerifyWebhook authenticates a Finish Notify with DANA's public key over
// POST:<path>:hex(sha256(compact body)):X-TIMESTAMP and parses it. path is the request path as DANA called it.
//
// shortcut: no replay window (DANA documents no skew) and none of the SDK's proxy-artifact body repairs
// (escaped quotes, wrapped keys); callers must confirm every status by inquiry before applying it, so a replayed
// or reordered notification cannot move money. Check against a captured sandbox callback (docs/integrations/dana.md).
func (a *Adapter) VerifyWebhook(h http.Header, path string, body []byte) (gateway.WebhookEvent, error) {
	if a.pub == nil {
		return gateway.WebhookEvent{}, &gateway.SignatureError{Reason: "no_public_key"}
	}
	sigB64, ts := h.Get("X-SIGNATURE"), h.Get("X-TIMESTAMP")
	if sigB64 == "" || ts == "" {
		return gateway.WebhookEvent{}, &gateway.SignatureError{Reason: "missing_signature_header"}
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return gateway.WebhookEvent{}, &gateway.SignatureError{Reason: "signature_not_base64"}
	}
	var min bytes.Buffer
	if json.Compact(&min, body) != nil {
		return gateway.WebhookEvent{}, errors.New("dana webhook: malformed body")
	}
	// DANA's SDK tries the exact received bytes first ("signature may also use the exact request bytes as
	// received") and the minified form after; either may be what was signed.
	bodies := [][]byte{body, min.Bytes()}
	// DANA's SDK verifies against the path of the request it received, so the registered URL path is the expected
	// one; the documented /v1.0/debit/notify is accepted as well (both are DANA-signed paths, nothing else is).
	verified := false
	for _, p := range []string{path, pathNotify} {
		for _, b := range bodies {
			h256 := sha256.Sum256([]byte(stringToSign(http.MethodPost, p, b, ts)))
			if rsa.VerifyPKCS1v15(a.pub, crypto.SHA256, h256[:], sig) == nil {
				verified = true
			}
		}
	}
	if !verified {
		return gateway.WebhookEvent{}, &gateway.SignatureError{Reason: "signature_mismatch"}
	}

	var n notify
	if json.Unmarshal(body, &n) != nil {
		return gateway.WebhookEvent{}, errors.New("dana webhook: malformed body")
	}
	ref, ok := LongRef(n.OriginalPartnerReferenceNo)
	if !ok || n.LatestTransactionStatus == "" {
		return gateway.WebhookEvent{}, errors.New("dana webhook: missing or unknown payment reference")
	}
	if n.MerchantID != a.cfg.MerchantID {
		return gateway.WebhookEvent{}, errors.New("dana webhook: merchant id does not match this account")
	}
	amt, ok := parseMoney(n.Amount.Value)
	if !ok {
		return gateway.WebhookEvent{}, fmt.Errorf("dana webhook: unsupported amount %q", n.Amount.Value)
	}
	st, mapped := a.MapStatus(n.LatestTransactionStatus)
	return gateway.WebhookEvent{
		PaymentRef: ref, RawStatus: n.LatestTransactionStatus, Status: st, StatusOK: mapped,
		Amount: amt, Currency: n.Amount.Currency,
		DedupKey: Code + ":webhook:" + n.OriginalPartnerReferenceNo + ":" + n.LatestTransactionStatus,
	}, nil
}

var _ gateway.WebhookVerifier = (*Adapter)(nil)
