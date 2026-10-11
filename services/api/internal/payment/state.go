// Package payment is the gateway-neutral payment domain: creation with a fee
// snapshot, the status state machine, and atomic ledger posting on success.
// It must not import any concrete gateway package (PRD §0 #11).
package payment

import "errors"

type Status string

const (
	Created   Status = "created"
	Pending   Status = "pending"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Expired   Status = "expired"
	Cancelled Status = "cancelled"
	Unknown   Status = "unknown" // ambiguous outcome: needs inquiry/reconciliation, NOT a failure (PRD §0 #3)
)

// Source says where a status claim came from (PRD §0 #2).
type Source string

const (
	SourceInternal        Source = "internal"         // our own bookkeeping; may never claim a final status
	SourceGatewayResponse Source = "gateway_response" // synchronous, definitive reply to a request we made
	SourceWebhook         Source = "webhook"          // validated gateway callback
	SourceInquiry         Source = "inquiry"          // official status query
)

var (
	ErrInvalidTransition = errors.New("payment: invalid status transition")
	ErrSourceNotAllowed  = errors.New("payment: source may not set this status")
)

// IsFinal reports terminal statuses. `unknown` is deliberately not final.
func (s Status) IsFinal() bool {
	switch s {
	case Succeeded, Failed, Expired, Cancelled:
		return true
	}
	return false
}

var allowed = map[Status][]Status{
	Created: {Pending, Failed, Cancelled, Unknown}, // create may be rejected, or time out -> unknown
	Pending: {Succeeded, Failed, Expired, Cancelled, Unknown},
	Unknown: {Pending, Succeeded, Failed, Expired},
}

// CanTransition reports whether from -> to is a legal move (from != to).
func CanTransition(from, to Status) bool {
	for _, t := range allowed[from] {
		if t == to {
			return true
		}
	}
	return false
}

// SourceMaySet enforces invariant #2: a final status can only come from a
// validated webhook, an official inquiry, or a definitive synchronous gateway
// reply; never from our own timers, the client, or a redirect.
func SourceMaySet(src Source, to Status) bool {
	switch src {
	case SourceWebhook, SourceInquiry, SourceGatewayResponse:
		return true
	case SourceInternal:
		return !to.IsFinal()
	}
	return false
}
