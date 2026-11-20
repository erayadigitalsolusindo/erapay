package payment

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// ErrNotPurgeable: the payment moved (or may still move) money, so it must stay.
var ErrNotPurgeable = errors.New("payment: cannot be deleted")

// purgeable is the single definition of "never moved money": a payment that failed, expired or was cancelled,
// or one that never reached a gateway (no reference) while still created/unknown, and that no ledger entry,
// refund, settlement or QRISGate record points at. pending/succeeded and anything a gateway may still settle stay.
const purgeable = `(p.status IN ('failed','expired','cancelled') OR (p.status IN ('created','unknown') AND p.gateway_reference IS NULL))
	AND NOT EXISTS (SELECT 1 FROM ledger_transactions l WHERE l.source_id = p.id::text)
	AND NOT EXISTS (SELECT 1 FROM refunds x WHERE x.payment_id = p.id)
	AND NOT EXISTS (SELECT 1 FROM settlement_items s WHERE s.payment_id = p.id)
	AND NOT EXISTS (SELECT 1 FROM qrisgate_topups q WHERE q.payment_id = p.id)
	AND NOT EXISTS (SELECT 1 FROM qrisgate_claims c WHERE c.payment_id = p.id)`

// PlatformRow is one payment in the platform's cross-tenant list.
type PlatformRow struct {
	ID                string    `json:"id"`
	MerchantID        string    `json:"merchant_id"`
	MerchantName      string    `json:"merchant_name"`
	MerchantReference string    `json:"merchant_reference"`
	Amount            int64     `json:"amount"`
	Currency          string    `json:"currency"`
	Status            Status    `json:"status"`
	Gateway           string    `json:"gateway"`
	CreatedAt         time.Time `json:"created_at"`
	Deletable         bool      `json:"deletable"`
}

// PlatformList lists the newest payments across all tenants, optionally narrowed by status and by q
// (payment id prefix or merchant reference). Platform-only: the caller must have passed the platform guard.
func (s *Service) PlatformList(ctx context.Context, status Status, q string, limit int) ([]PlatformRow, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if status != "" && !ValidStatus(status) {
		return nil, ErrValidation
	}
	rows, err := s.db.Query(ctx, `
		SELECT p.id::text, p.merchant_id::text, m.display_name, p.merchant_reference, p.amount, p.currency, p.status, p.gateway, p.created_at,
		       `+purgeable+`
		FROM payments p JOIN merchants m ON m.id = p.merchant_id
		WHERE ($1 = '' OR p.status = $1)
		  AND ($2 = '' OR p.id::text LIKE $2 || '%' OR p.merchant_reference = $2)
		ORDER BY p.created_at DESC, p.id DESC LIMIT $3`, string(status), strings.TrimSpace(q), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlatformRow{}
	for rows.Next() {
		var r PlatformRow
		if err := rows.Scan(&r.ID, &r.MerchantID, &r.MerchantName, &r.MerchantReference, &r.Amount, &r.Currency, &r.Status, &r.Gateway, &r.CreatedAt, &r.Deletable); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Purge hard-deletes a payment that never moved money, with its events and attempts, and records who did it and
// why in the append-only audit log (with a snapshot of what was deleted) in the SAME transaction.
func (s *Service) Purge(ctx context.Context, id, actorID, reason, requestID string) error {
	reason = strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(reason); n < 3 || n > 500 {
		return ErrValidation
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var snap struct {
		merchantID, ref, status, gateway, gwRef string
		amount                                  int64
		ok                                      bool
	}
	err = tx.QueryRow(ctx, `
		SELECT p.merchant_id::text, p.merchant_reference, p.status, p.gateway, COALESCE(p.gateway_reference,''), p.amount, `+purgeable+`
		FROM payments p WHERE p.id::text = $1 FOR UPDATE`, id).
		Scan(&snap.merchantID, &snap.ref, &snap.status, &snap.gateway, &snap.gwRef, &snap.amount, &snap.ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !snap.ok {
		return ErrNotPurgeable
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "payment.purge", EntityType: "payment", EntityID: id,
		RequestID: requestID, Metadata: map[string]any{"merchant_id": snap.merchantID, "merchant_reference": snap.ref, "status": snap.status,
			"gateway": snap.gateway, "gateway_reference": snap.gwRef, "amount": snap.amount, "reason": reason}}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('erapay.purge_payment', 'on', true)`); err != nil { // transaction-local
		return err
	}
	for _, q := range []string{`DELETE FROM payment_events WHERE payment_id = $1::uuid`, `DELETE FROM payment_attempts WHERE payment_id = $1::uuid`,
		`DELETE FROM payments WHERE id = $1::uuid`} {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
