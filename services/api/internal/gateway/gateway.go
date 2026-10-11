// Package gateway defines the gateway-neutral adapter contract and registry
// (PRD §0 #11, architecture.md). Domain code depends on this package only, never
// on a concrete gateway package. Concrete adapters live in subpackages and are
// wired in cmd/api.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status is the normalized payment status an adapter maps a raw gateway status to.
// Values intentionally equal the payment domain's status strings.
type Status string

const (
	StatusPending   Status = "pending"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)

// Capabilities are read by the API/UI instead of hard-coding per gateway.
type Capabilities struct {
	PaymentMethods   []string
	Inquiry          bool
	Cancel           bool
	Refund           bool
	PartialRefund    bool
	SettlementReport bool
	Disbursement     bool
}

func (c Capabilities) SupportsMethod(m string) bool {
	for _, x := range c.PaymentMethods {
		if x == m {
			return true
		}
	}
	return false
}

// CreateRequest: Reference is our opaque payment ID (the gateway's partner reference).
// AccountRef is the merchant's identifier AT the gateway (merchant_gateway_accounts.gateway_ref).
type CreateRequest struct {
	Reference   string
	AccountRef  string
	Method      string
	Amount      int64 // minor units of Currency
	Currency    string
	ExpiresAt   time.Time
	Description string
}

type CreateResult struct {
	GatewayReference string
	QRPayload        string // provider_defined
	RawStatus        string
	ResponseCode     string // safe to store/log
}

type QueryRequest struct {
	Reference        string
	GatewayReference string
	AccountRef       string
}

type QueryResult struct {
	RawStatus    string
	Amount       int64 // -1 when the gateway did not return a usable amount
	Currency     string
	PaidAt       *time.Time
	ResponseCode string
}

type CancelRequest struct {
	Reference        string
	GatewayReference string
	AccountRef       string
}

// Kind classifies a failed gateway call. The distinction is financially critical:
// Rejected = the gateway definitively did not accept it; Ambiguous = it may have
// been processed, so the outcome is UNKNOWN and must be resolved by inquiry
// (PRD §0 #3: never treat as failed, never blindly retry create).
type Kind int

const (
	Rejected Kind = iota + 1
	Ambiguous
)

type Error struct {
	Kind       Kind
	Code       string // gateway response code, safe to expose to ops
	HTTPStatus int
	Retryable  bool
	Message    string // sanitized; never contains secrets or raw bodies
}

func (e *Error) Error() string {
	k := "rejected"
	if e.Kind == Ambiguous {
		k = "ambiguous"
	}
	return fmt.Sprintf("gateway %s (http %d, code %q): %s", k, e.HTTPStatus, e.Code, e.Message)
}

func IsAmbiguous(err error) bool { var e *Error; return errors.As(err, &e) && e.Kind == Ambiguous }
func IsRejected(err error) bool  { var e *Error; return errors.As(err, &e) && e.Kind == Rejected }

var ErrNotSupported = errors.New("gateway: operation not supported")

// Adapter is the contract every gateway implements. Webhook verification/parsing and
// refunds join the interface when their official specs are confirmed (docs/integrations).
type Adapter interface {
	Code() string
	Capabilities() Capabilities
	CreatePayment(ctx context.Context, r CreateRequest) (CreateResult, error)
	GetPayment(ctx context.Context, r QueryRequest) (QueryResult, error)
	CancelPayment(ctx context.Context, r CancelRequest) error
	// MapStatus maps a raw gateway status to a normalized one. ok=false means
	// "no confirmed mapping": callers must leave the payment unchanged.
	MapStatus(raw string) (Status, bool)
}

// Registry maps gateway code -> adapter. Only adapters explicitly registered exist.
type Registry struct{ m map[string]Adapter }

func NewRegistry() *Registry { return &Registry{m: map[string]Adapter{}} }

func (r *Registry) Register(a Adapter) {
	if _, dup := r.m[a.Code()]; dup {
		panic("gateway: duplicate adapter " + a.Code())
	}
	r.m[a.Code()] = a
}

func (r *Registry) Get(code string) (Adapter, bool) { a, ok := r.m[code]; return a, ok }
