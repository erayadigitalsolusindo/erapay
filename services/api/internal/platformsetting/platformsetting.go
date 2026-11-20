// Package platformsetting stores platform-wide integer settings that DEWA edits in the panel.
// Only whitelisted keys exist (CHECK in the table); every change is audited with a reason.
package platformsetting

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// QRISGateFee is the per-transaction deduction (IDR) from the tenant's balance for a succeeded QRISGate payment.
const QRISGateFee = "qrisgate_fee"

// MaxFee bounds a fee so a typo cannot drain balances (Rp1.000.000 per transaction).
const MaxFee = 1_000_000

var ErrInvalid = errors.New("platformsetting: invalid value")

type Service struct{ db *pgxpool.Pool }

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

func (s *Service) Int(ctx context.Context, key string) (int64, error) {
	var v int64
	err := s.db.QueryRow(ctx, `SELECT int_value FROM platform_settings WHERE key = $1`, key).Scan(&v)
	return v, err
}

// SetFee changes qrisgate_fee. Reason (>= 3 chars) is mandatory and lands in the audit log.
func (s *Service) SetFee(ctx context.Context, value int64, reason, actorID, requestID string) error {
	if value < 0 || value > MaxFee || utf8.RuneCountInString(reason) < 3 || utf8.RuneCountInString(reason) > 500 {
		return ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var old int64
	if err := tx.QueryRow(ctx, `SELECT int_value FROM platform_settings WHERE key = $1 FOR UPDATE`, QRISGateFee).Scan(&old); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE platform_settings SET int_value = $2, updated_by = NULLIF($3,'')::uuid, updated_at = now() WHERE key = $1`,
		QRISGateFee, value, actorID); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "platform_setting.update", EntityType: "platform_setting",
		EntityID: QRISGateFee, RequestID: requestID, Metadata: map[string]any{"old": old, "new": value, "reason": reason}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
