package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

var (
	// ErrNoSandbox: the merchant has not enabled sandbox.
	ErrNoSandbox = errors.New("auth: sandbox is not enabled for this merchant")
	// ErrSwitchNotAllowed: only a normal merchant session can switch (not platform or impersonation sessions).
	ErrSwitchNotAllowed = errors.New("auth: this session cannot switch between sandbox and live")
)

// SwitchSandbox points the caller's session at the merchant's sandbox twin (toSandbox) or back at the real
// merchant. Nobody is linked to a twin: a twin session takes its permissions from the user's membership in the
// parent merchant (see Authenticate), so membership removal ends it like any other session. Returns the
// merchant id the session now works on.
func (s *Service) SwitchSandbox(ctx context.Context, p *Principal, toSandbox bool, requestID string) (string, error) {
	if p == nil || p.MerchantID == "" || p.ImpersonatorID != "" {
		return "", ErrSwitchNotAllowed
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var parent string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(parent_id, id)::text FROM merchants WHERE id::text = $1`, p.MerchantID).Scan(&parent); err != nil {
		return "", err
	}
	target := parent
	if toSandbox {
		if err := tx.QueryRow(ctx, `SELECT id::text FROM merchants WHERE parent_id::text = $1`, parent).Scan(&target); errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNoSandbox
		} else if err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET merchant_id = $2 WHERE id::text = $1 AND revoked_at IS NULL`, p.SessionID, target); err != nil {
		return "", err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: p.UserID, Action: "session.switch_sandbox", EntityType: "session", EntityID: p.SessionID,
		RequestID: requestID, Metadata: map[string]any{"from_merchant_id": p.MerchantID, "to_merchant_id": target, "sandbox": toSandbox}}); err != nil {
		return "", err
	}
	return target, tx.Commit(ctx)
}
