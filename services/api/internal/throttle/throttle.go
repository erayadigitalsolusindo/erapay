// Package throttle is a small DB-backed rate limiter for unauthenticated, email-sending endpoints
// (sign-up, resend verification, forgot password). Attempts are recorded before the work is done,
// so failed and rejected requests count too.
package throttle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrIPLimited means the caller's IP is over its limit; the endpoint should answer 429.
var ErrIPLimited = errors.New("throttle: too many requests from this address")

type Limits struct {
	PerIP    int // 0 = no per-IP limit
	PerEmail int // 0 = no per-email limit
	Window   time.Duration
}

// EmailKey hashes the normalized email so no raw address is stored in the throttle table.
func EmailKey(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:])
}

// Reserve records an attempt of the given kind. It returns ErrIPLimited when the IP is over its limit
// (nothing is recorded then). emailLimited reports that this email is over its limit: the caller must
// then skip the side effect (no email) but answer exactly like success, so the limit cannot be used
// to probe which addresses exist. Parallel callers are serialized by advisory locks (fixed order).
func Reserve(ctx context.Context, db *pgxpool.Pool, kind, ip, emailKey string, l Limits) (emailLimited bool, err error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('throttle:'||$1||':i:'||$2, 0))`, kind, ip); err != nil {
		return false, err
	}
	if emailKey != "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('throttle:'||$1||':e:'||$2, 0))`, kind, emailKey); err != nil {
			return false, err
		}
	}
	var byIP, byEmail int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE ip = $2), count(*) FILTER (WHERE email_key = NULLIF($3,''))
		FROM register_attempts
		WHERE kind = $1 AND created_at > now() - make_interval(secs => $4) AND (ip = $2 OR email_key = NULLIF($3,''))`,
		kind, ip, emailKey, l.Window.Seconds()).Scan(&byIP, &byEmail); err != nil {
		return false, err
	}
	if l.PerIP > 0 && byIP >= l.PerIP {
		return false, ErrIPLimited
	}
	if _, err := tx.Exec(ctx, `INSERT INTO register_attempts (kind, ip, email_key) VALUES ($1,$2,NULLIF($3,''))`, kind, ip, emailKey); err != nil {
		return false, err
	}
	// Housekeeping: rows older than a day can never count again (windows are an hour), so each caller
	// trims its own old rows and the table stays bounded without a background job.
	if _, err := tx.Exec(ctx, `DELETE FROM register_attempts WHERE kind = $1 AND created_at < now() - interval '1 day' AND (ip = $2 OR email_key = NULLIF($3,''))`,
		kind, ip, emailKey); err != nil {
		return false, err
	}
	return l.PerEmail > 0 && emailKey != "" && byEmail >= l.PerEmail, tx.Commit(ctx)
}
