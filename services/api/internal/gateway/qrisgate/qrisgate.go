// Package qrisgate is EraPay's OWN mutation-detection QRIS gateway (it is not a client of any third-party service).
// The tenant's money goes straight to their own bank/e-wallet (a Direct gateway: no clearing, settlement, refund or
// payout). Creating a payment turns the tenant's static QRIS into a dynamic one for base + a unique code (the pay
// amount); a payment is recognised when the Android listener (apps/listener-android) forwards a signed notification
// whose amount equals that pay amount. See docs/integrations/qrisgate.md for the security model and its limits.
//
// All state lives behind Port (implemented by internal/qrisgate), so the adapter itself holds no secret and no SQL.
package qrisgate

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/qrisgate"
)

const (
	Code       = "qrisgate"
	methodQRIS = "QRIS"
)

// Port is the adapter's only dependency (satisfied by *qrisgate.Service).
type Port interface {
	Claim(ctx context.Context, merchantID, paymentID string, base int64, expiresAt time.Time) (payAmount int64, qrisPayload string, err error)
	ClaimExpired(ctx context.Context, paymentID string) (found, expired bool, err error)
	ListenerSecret(ctx context.Context, merchantID string) (string, error)
	MatchNotification(ctx context.Context, merchantID, deliveryID, pkg, title, text string, amount int64, postedAt time.Time) (paymentID string, base int64, matched bool, err error)
}

type Config struct {
	Port Port
	Now  func() time.Time
}

type Adapter struct{ cfg Config }

func New(cfg Config) (*Adapter, error) {
	if cfg.Port == nil {
		return nil, errors.New("qrisgate: port is required")
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

// CreatePayment reserves the pay amount and returns the dynamic QRIS. Nothing leaves our own database, so a failure
// before the claim commits changes nothing; the claim is idempotent per payment, so a retry returns the same QR.
func (a *Adapter) CreatePayment(ctx context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	if r.Method != methodQRIS {
		return gateway.CreateResult{}, gateway.ErrNotSupported
	}
	if !uuidRe.MatchString(r.AccountRef) || !uuidRe.MatchString(r.Reference) || r.Amount <= 0 || r.Currency != "IDR" || r.ExpiresAt.IsZero() {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid create request"}
	}
	_, payload, err := a.cfg.Port.Claim(ctx, r.AccountRef, r.Reference, r.Amount, r.ExpiresAt)
	switch {
	case errors.Is(err, qrisgate.ErrNoQRIS):
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Code: "no_qris", Message: "the merchant has not saved a valid static QRIS"}
	case errors.Is(err, qrisgate.ErrNoFreeCode):
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Code: "no_free_code", Message: "too many open payments of this amount; try again shortly"}
	case err != nil:
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Ambiguous, Retryable: true, Message: "could not reserve the pay amount"}
	}
	return gateway.CreateResult{GatewayReference: "own-" + r.Reference, QRPayload: payload, RawStatus: "PENDING", ResponseCode: "claimed"}, nil
}

// GetPayment: there is no upstream to ask. A payment is paid by a notification (webhook path); inquiry only reports
// that its QR ran out, which lets the poller close it as expired. It never claims success.
func (a *Adapter) GetPayment(ctx context.Context, r gateway.QueryRequest) (gateway.QueryResult, error) {
	if !uuidRe.MatchString(r.Reference) {
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Rejected, Message: "invalid query request"}
	}
	found, expired, err := a.cfg.Port.ClaimExpired(ctx, r.Reference)
	switch {
	case err != nil:
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, Retryable: true, Message: "could not read the claim"}
	case !found:
		return gateway.QueryResult{}, &gateway.Error{Kind: gateway.Ambiguous, Message: "no claim for this payment"}
	case expired:
		return gateway.QueryResult{RawStatus: "EXPIRED", Amount: -1, Currency: "IDR"}, nil
	}
	return gateway.QueryResult{RawStatus: "PENDING", Amount: -1, Currency: "IDR"}, nil
}

func (a *Adapter) CancelPayment(context.Context, gateway.CancelRequest) error {
	return gateway.ErrNotSupported
}

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

var _ gateway.Adapter = (*Adapter)(nil)
