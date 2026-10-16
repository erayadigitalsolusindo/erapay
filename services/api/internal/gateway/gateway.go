// Package gateway defines the gateway-neutral adapter contract and registry
// (PRD §0 #11, architecture.md). Domain code depends on this package only, never
// on a concrete gateway package. Concrete adapters live in subpackages and are
// wired in cmd/api.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
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

// ConnectionStep is one check of a connection test. Message is safe to show to an operator
// (never a secret or a raw gateway body).
type ConnectionStep struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// ConnectionTester is implemented by adapters that can verify their credentials and a merchant's
// account reference without moving money. With deep=false only credentials are checked (no
// merchant data is sent). With deep=true the adapter also exercises accountRef the way a real
// payment would (e.g. a short-lived QR that is cancelled right away) and must clean up after itself.
type ConnectionTester interface {
	TestConnection(ctx context.Context, accountRef string, deep bool) []ConnectionStep
}

// ErrInvalidSignature: the notification is not authentic. Callers answer 4xx and change nothing.
var ErrInvalidSignature = errors.New("gateway: invalid webhook signature")

// WebhookEvent is an authenticated, normalized inbound payment notification.
// It carries no PII: adapters must drop payer data while parsing.
type WebhookEvent struct {
	PaymentRef string // our payment ID (the gateway's partner/invoice reference)
	RawStatus  string // gateway's own status string
	Status     Status // normalized; meaningful only when StatusOK
	StatusOK   bool   // false = no confirmed mapping: record, never apply
	Amount     int64  // minor units
	Currency   string
	DedupKey   string // unique per notification (gateway-scoped)
}

// WebhookVerifier is implemented by adapters whose notification format and signature
// are confirmed. path is the request path exactly as the gateway called it.
// A verified-but-unparseable payload returns a non-ErrInvalidSignature error.
type WebhookVerifier interface {
	VerifyWebhook(h http.Header, path string, body []byte) (WebhookEvent, error)
}

// TransferStatus is the normalized payout status an adapter maps a raw transfer status to.
// Distinct from payment Status: a payout that fails or is returned is its own state machine.
type TransferStatus string

const (
	TransferSucceeded TransferStatus = "succeeded"
	TransferPending   TransferStatus = "pending"
	TransferFailed    TransferStatus = "failed"
	TransferRefunded  TransferStatus = "refunded" // beneficiary was credited back; separate state machine
)

// Disburser is implemented by adapters whose payout endpoints are confirmed
// (docs/integrations/<gateway>.md). The payout executor depends on this contract, never
// on a concrete adapter, so a second gateway can join or replace it (PRD §0 #11).
// Flow: InquiryBankAccount (mandatory before a transfer, partner-paid per success so
// callers cache it) -> TransferBank -> TransferStatus (recovery for ambiguous outcomes).
type Disburser interface {
	// Code is the gateway code (matching the registered adapter), recorded on the payout.
	Code() string
	// InquiryBankAccount validates the beneficiary and returns the gateway's session id,
	// which TransferBank requires, plus the fee the gateway will charge for the transfer.
	InquiryBankAccount(ctx context.Context, r BankInquiryRequest) (BankInquiryResult, error)
	// TransferBank moves funds. Reference (the gateway's partner reference) must be unique
	// per attempt; a duplicate is ambiguous and must never be re-sent blindly (PRD §0 #3).
	TransferBank(ctx context.Context, r TransferRequest) (TransferResult, error)
	// TransferStatus asks for the current outcome of a transfer.
	TransferStatus(ctx context.Context, r TransferStatusRequest) (TransferStatusResult, error)
	// MapTransferStatus maps a raw transfer status; ok=false = no confirmed mapping,
	// callers leave the payout unchanged.
	MapTransferStatus(raw string) (TransferStatus, bool)
}

// CustomerNumber is the remitter's phone as the gateway expects it. Bank codes are the
// gateway's own (see docs/integrations); AccountNo is the account number, or the
// beneficiary's phone number for e-wallets.

type BankInquiryRequest struct {
	CustomerNumber string // remitter phone (adapter normalizes to the gateway's format)
	AccountNo      string // beneficiary account number, or phone for e-wallets
	AccountName    string // beneficiary name as the merchant stated it
	BankCode       string // gateway bank code
	BankName       string // optional, when the domain already knows it
	Amount         int64
	Currency       string
	SenderCountry  string // ISO 3166-1 alpha-2
	ChannelCode    string // "07" bank deposit | "11" BI-FAST; "" = gateway default
	Reference      string // optional partner reference (not the payout reference)
}

type BankInquiryResult struct {
	SessionID     string // required by TransferBank; gateway-scoped invoice id
	AccountName   string // name the gateway reports for the account
	BankName      string
	BankShortName string
	FeeAmount     int64 // -1 when the gateway did not return one
	ResponseCode  string
}

type TransferRequest struct {
	Reference      string // partner reference (unique per attempt = gateway idempotency)
	CustomerNumber string
	SessionID      string // from InquiryBankAccount
	AccountNo      string
	BankCode       string
	BankName       string // optional
	Amount         int64
	Currency       string
	// Beneficiary: the merchant receiving the payout.
	BeneficiaryFirstName   string
	BeneficiaryLastName    string
	BeneficiaryPhone       string
	BeneficiaryAccountName string // as returned by the inquiry
	// Sender: the platform entity remitting the funds.
	SenderCountry    string // ISO 3166-1 alpha-2
	SenderFirstName  string
	SenderLastName   string
	SenderPersonalID string // national id; for a business remitter this is [?] per gateway
	SenderIDType     string // gateway-defined, e.g. "KTP" | "PASSPORT"
	ChannelCode      string // "07" | "11"; "" = gateway default
	Remark           string
}

type TransferResult struct {
	GatewayReference string
	TransactionDate  *time.Time
	ResponseCode     string
}

type TransferStatusRequest struct {
	OriginalPartnerReference string // the transfer's Reference, when known
	OriginalReference        string // the gateway reference from TransferResult, when known
	AccountNo                string
	BankCode                 string
	ServiceCode              string // service code of the original request; gateway-specific, may be ""
}

type TransferStatusResult struct {
	RawStatus       string
	Amount          int64 // -1 when the gateway did not return a usable amount
	TransactionDate *time.Time
	ResponseCode    string
}

// Registry maps gateway code -> adapter. Only adapters explicitly registered exist.
// Safe for concurrent use: Replace swaps an adapter while requests are in flight.
type Registry struct {
	mu sync.RWMutex
	m  map[string]Adapter
}

func NewRegistry() *Registry { return &Registry{m: map[string]Adapter{}} }

func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.m[a.Code()]; dup {
		panic("gateway: duplicate adapter " + a.Code())
	}
	r.m[a.Code()] = a
}

// Replace registers a, or swaps the adapter already registered under the same code (credential
// rotation without a restart). Callers that already hold the old adapter finish with it.
func (r *Registry) Replace(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[a.Code()] = a
}

func (r *Registry) Get(code string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.m[code]
	return a, ok
}

// Codes lists the registered gateway codes, sorted.
func (r *Registry) Codes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.m))
	for c := range r.m {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
