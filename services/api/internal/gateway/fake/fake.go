// Package fake is the SANDBOX payment gateway: QRIS with no upstream, no credentials and no money. It exists
// only in the sandbox registry of a production server (ERAPAY_SANDBOX_FAKE=1) and is driven by the simulate
// endpoint (docs/integrations/fake.md). It keeps no state: the payment row is the truth, and a payment moves
// only when the simulator applies an outcome, exactly like a verified webhook would.
package fake

import (
	"context"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/gateway"
)

const Code = "fake"

// Raw statuses the simulator uses; MapStatus is the only place they become normalized statuses.
const (
	RawPending = "PENDING"
	RawPaid    = "PAID"
	RawExpired = "EXPIRED"
	RawFailed  = "FAILED"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (*Adapter) Code() string { return Code }

func (*Adapter) Capabilities() gateway.Capabilities {
	return gateway.Capabilities{PaymentMethods: []string{"QRIS"}, Inquiry: true, Cancel: true}
}

func (*Adapter) CreatePayment(_ context.Context, r gateway.CreateRequest) (gateway.CreateResult, error) {
	if r.Reference == "" || r.Amount <= 0 {
		return gateway.CreateResult{}, &gateway.Error{Kind: gateway.Rejected, Code: "FAKE_INVALID", Message: "reference and positive amount required"}
	}
	return gateway.CreateResult{GatewayReference: "FAKE-" + r.Reference, QRPayload: "ERAPAY-SANDBOX-QR:" + r.Reference,
		RawStatus: RawPending, ResponseCode: "FAKE_OK"}, nil
}

// GetPayment always answers "still pending": the outcome is applied by the simulator, never discovered by polling.
func (*Adapter) GetPayment(context.Context, gateway.QueryRequest) (gateway.QueryResult, error) {
	return gateway.QueryResult{RawStatus: RawPending, Amount: -1, ResponseCode: "FAKE_OK"}, nil
}

func (*Adapter) CancelPayment(context.Context, gateway.CancelRequest) error { return nil }

func (*Adapter) MapStatus(raw string) (gateway.Status, bool) {
	switch raw {
	case RawPending:
		return gateway.StatusPending, true
	case RawPaid:
		return gateway.StatusSucceeded, true
	case RawExpired:
		return gateway.StatusExpired, true
	case RawFailed:
		return gateway.StatusFailed, true
	}
	return "", false
}

var _ gateway.Simulator = (*Adapter)(nil)

// SimulateEvent builds the notification the simulator applies for an outcome. The dedup key is per payment and
// outcome, so repeating a simulation is a no-op.
func (a *Adapter) SimulateEvent(paymentID string, amount int64, currency, outcome string) (gateway.WebhookEvent, error) {
	raw := map[string]string{"paid": RawPaid, "expired": RawExpired, "failed": RawFailed}[outcome]
	if raw == "" {
		return gateway.WebhookEvent{}, gateway.ErrUnknownOutcome
	}
	st, _ := a.MapStatus(raw)
	return gateway.WebhookEvent{PaymentRef: paymentID, RawStatus: raw, Status: st, StatusOK: true, Amount: amount, Currency: currency,
		DedupKey: "fake:simulate:" + paymentID + ":" + outcome}, nil
}
