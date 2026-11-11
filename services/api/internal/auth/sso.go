package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// Masuk satu-klik dari ARUS: ARUS (server-to-server, token provisioning) meminta kode untuk
// Owner tenant yang tertaut; browser menukarnya sekali jadi sesi merchant biasa.
// Keputusan produk: ARUS sudah mengautentikasi Owner, jadi SSO tidak meminta MFA EraPay lagi (ekosistem).

var (
	ErrSSOUnavailable = errors.New("sso not available for this tenant")
	ErrSSOInvalidCode = errors.New("sso code invalid or expired")
	ssoCodeTTL        = 60 * time.Second
)

// IssueARUSSSOCode returns a single-use code for the owner of the merchant linked to an ARUS tenant.
func (s *Service) IssueARUSSSOCode(ctx context.Context, tenantID uuid.UUID, requestID string) (string, time.Duration, error) {
	var uid, mid string
	err := s.db.QueryRow(ctx, `
		SELECT l.owner_user_id::text, l.merchant_id::text
		FROM merchant_external_links l
		JOIN users u ON u.id = l.owner_user_id AND u.status = 'active'
		JOIN merchant_users mu ON mu.merchant_id = l.merchant_id AND mu.user_id = l.owner_user_id
		JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
		WHERE l.source = 'arus' AND l.source_tenant_id = $1`, tenantID).Scan(&uid, &mid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrSSOUnavailable
	}
	if err != nil {
		return "", 0, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", 0, err
	}
	code := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := s.db.Exec(ctx, `INSERT INTO sso_codes (code_hash, user_id, merchant_id, expires_at) VALUES ($1, $2, $3, $4)`,
		hashToken(code), uid, mid, s.now().Add(ssoCodeTTL)); err != nil {
		return "", 0, err
	}
	_, _ = s.db.Exec(ctx, `DELETE FROM sso_codes WHERE expires_at < now() - interval '1 day'`) // housekeeping; best effort
	return code, ssoCodeTTL, audit.Record(ctx, s.db, audit.Entry{ActorID: uid, Action: "auth.sso.issued",
		EntityType: "merchant", EntityID: mid, RequestID: requestID, Metadata: map[string]any{"source": "arus"}})
}

// ExchangeSSOCode burns the code and opens a normal merchant session for its user.
func (s *Service) ExchangeSSOCode(ctx context.Context, code string, in LoginInput) (string, *Principal, error) {
	var uid, mid string
	err := s.db.QueryRow(ctx, `UPDATE sso_codes SET used_at = now()
		WHERE code_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id::text, merchant_id::text`, hashToken(code)).Scan(&uid, &mid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrSSOInvalidCode
	}
	if err != nil {
		return "", nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var sid string
	if err := tx.QueryRow(ctx,
		`INSERT INTO sessions (user_id, merchant_id, token_hash, expires_at, ip, user_agent)
		 SELECT u.id, $2, $3, $4, $5, $6 FROM users u WHERE u.id = $1 AND u.status = 'active' RETURNING id::text`,
		uid, mid, hashToken(token), s.now().Add(s.cfg.SessionTTL), in.IP, in.UserAgent).Scan(&sid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, ErrSSOInvalidCode
		}
		return "", nil, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: uid, Action: "auth.login.success",
		EntityType: "session", EntityID: sid, RequestID: in.RequestID,
		Metadata: map[string]any{"ip": in.IP, "merchant_id": mid, "via": "arus_sso"}}); err != nil {
		return "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	p, err := s.Authenticate(ctx, token)
	return token, p, err
}
