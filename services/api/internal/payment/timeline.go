package payment

import (
	"context"
	"errors"
	"time"
)

// Event is one recorded transition attempt for a payment.
//
// Rows with applied=false are late or conflicting gateway reports (for example
// "succeeded" arriving after the payment already expired). They are kept so
// reconciliation can see what the gateway claimed, but they never move money
// (PRD §0 #2): the timeline reports them, it does not re-apply them.
type Event struct {
	FromStatus       string    `json:"from_status"`
	ToStatus         string    `json:"to_status"`
	Applied          bool      `json:"applied"`
	Source           Source    `json:"source"`
	Reason           string    `json:"reason,omitempty"`
	RawGatewayStatus string    `json:"raw_gateway_status,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// MaxTimelineEvents bounds the response. A payment normally has a handful of
// transitions; the cap only guards against an unexpected burst, never normal use.
const MaxTimelineEvents = 500

// Events returns the append-only transition log for one payment, oldest first.
//
// Ownership is checked first so that a payment belonging to another tenant is
// indistinguishable from one that does not exist (PRD §0 #7). A payment that exists
// but has no transitions yet returns an empty slice, not an error: creating a payment
// writes no payment_events row, only the first transition does.
func (s *Service) Events(ctx context.Context, merchantID, paymentID string) ([]Event, error) {
	if merchantID == "" || paymentID == "" {
		return nil, errors.New("merchant and payment are required")
	}

	var mine bool
	if err := s.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM payments WHERE id = $1 AND merchant_id = $2)`,
		paymentID, merchantID).Scan(&mine); err != nil {
		return nil, err
	}
	if !mine {
		return nil, ErrNotFound
	}

	rows, err := s.db.Query(ctx, `
		SELECT from_status, to_status, applied, source,
		       COALESCE(reason,''), COALESCE(raw_gateway_status,''), created_at
		FROM payment_events
		WHERE payment_id = $1
		ORDER BY created_at, id
		LIMIT $2`, paymentID, MaxTimelineEvents)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Event{}
	for rows.Next() {
		var e Event
		var from, to, src string
		if err := rows.Scan(&from, &to, &e.Applied, &src, &e.Reason, &e.RawGatewayStatus, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.FromStatus, e.ToStatus, e.Source = from, to, Source(src)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
