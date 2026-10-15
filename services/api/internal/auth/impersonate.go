package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// Impersonasi DEWA: sesi merchant yang dibuka oleh DEWA. Sesi tetap atas nama user DEWA
// (jejak audit selalu menunjuk orang yang sebenarnya), tetapi tenant-nya satu merchant
// dan izinnya = izin role merchant_owner selama DEWA masih memegang rbac:manage.

var (
	ErrMerchantNotFound  = errors.New("merchant not found")
	ErrReasonRequired    = errors.New("impersonation reason required")
	ErrNotPlatformAdmin  = errors.New("platform admin required")
	minReasonLen         = 10
	impersonationTTL     = 60 * time.Minute
	impersonationRoleSQL = `ARRAY(SELECT rp.permission_code FROM roles r JOIN role_permissions rp ON rp.role_id = r.id
	               WHERE r.code = 'merchant_owner')`
)

// ImpersonationTTL is the absolute lifetime of an impersonation session (cookie Max-Age).
func ImpersonationTTL() time.Duration { return impersonationTTL }

type StartImpersonationInput struct {
	MerchantID string
	Reason     string
	IP         string
	UserAgent  string
	RequestID  string
}

// StartImpersonation opens a merchant-bound session for the calling DEWA. The reason is
// mandatory (>= 10 chars), the start is audited in the same transaction as the session,
// and any earlier impersonation session of the same DEWA is closed first so only one
// tenant can be operated at a time.
func (s *Service) StartImpersonation(ctx context.Context, dewa *Principal, in StartImpersonationInput) (string, *Principal, error) {
	if dewa == nil || dewa.MerchantID != "" || dewa.ImpersonatorID != "" || !dewa.Has("rbac:manage") {
		return "", nil, ErrNotPlatformAdmin
	}
	reason := strings.TrimSpace(in.Reason)
	if len([]rune(reason)) < minReasonLen || len(reason) > 500 {
		return "", nil, ErrReasonRequired
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

	var name string
	err = tx.QueryRow(ctx, `SELECT display_name FROM merchants WHERE id = $1`, in.MerchantID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrMerchantNotFound
	}
	if err != nil {
		return "", nil, err
	}

	// Satu impersonasi aktif per DEWA: tutup yang lama (dan catat penutupannya).
	rows, err := tx.Query(ctx, `
		UPDATE sessions SET revoked_at = now(), revoked_reason = 'impersonation_replaced'
		WHERE impersonator_id = $1 AND revoked_at IS NULL
		RETURNING id::text, merchant_id::text`, dewa.UserID)
	if err != nil {
		return "", nil, err
	}
	type closed struct{ sid, mid string }
	var old []closed
	for rows.Next() {
		var c closed
		if err := rows.Scan(&c.sid, &c.mid); err != nil {
			rows.Close()
			return "", nil, err
		}
		old = append(old, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	for _, c := range old {
		if err := audit.Record(ctx, tx, audit.Entry{ActorID: dewa.UserID, Action: "impersonation.end",
			EntityType: "merchant", EntityID: c.mid, RequestID: in.RequestID,
			Metadata: map[string]any{"session_id": c.sid, "cause": "replaced"}}); err != nil {
			return "", nil, err
		}
	}

	var sid string
	if err := tx.QueryRow(ctx,
		`INSERT INTO sessions (user_id, merchant_id, token_hash, expires_at, ip, user_agent, impersonator_id, impersonation_reason)
		 VALUES ($1, $2, $3, $4, $5, $6, $1, $7) RETURNING id::text`,
		dewa.UserID, in.MerchantID, hashToken(token), s.now().Add(impersonationTTL), in.IP, in.UserAgent, reason).Scan(&sid); err != nil {
		return "", nil, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: dewa.UserID, Action: "impersonation.start",
		EntityType: "merchant", EntityID: in.MerchantID, RequestID: in.RequestID,
		Metadata: map[string]any{"session_id": sid, "reason": reason, "merchant_name": name,
			"ip": in.IP, "expires_in_s": int(impersonationTTL.Seconds())}}); err != nil {
		return "", nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	p, err := s.Authenticate(ctx, token)
	return token, p, err
}

// MerchantSummary is the minimal tenant row the DEWA picker shows.
type MerchantSummary struct {
	ID               string    `json:"id"`
	DisplayName      string    `json:"display_name"`
	LegalName        string    `json:"legal_name"`
	OnboardingStatus string    `json:"onboarding_status"`
	CreatedAt        time.Time `json:"created_at"`
}

// SearchMerchants lists tenants for the impersonation picker (name / id prefix match).
func (s *Service) SearchMerchants(ctx context.Context, q string, limit int) ([]MerchantSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q = strings.TrimSpace(q)
	rows, err := s.db.Query(ctx, `
		SELECT id::text, display_name, legal_name, onboarding_status, created_at
		FROM merchants
		WHERE $1 = '' OR display_name ILIKE '%'||$1||'%' OR legal_name ILIKE '%'||$1||'%' OR id::text LIKE $1||'%'
		ORDER BY created_at DESC LIMIT $2`, escapeLike(q), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MerchantSummary])
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// AuditRow is one audit_logs entry as shown in the DEWA panel.
type AuditRow struct {
	ID         string         `json:"id"`
	ActorID    *string        `json:"actor_id"`
	ActorEmail *string        `json:"actor_email"`
	Action     string         `json:"action"`
	EntityType string         `json:"entity_type"`
	EntityID   *string        `json:"entity_id"`
	Metadata   map[string]any `json:"metadata"`
	RequestID  *string        `json:"request_id"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AuditFilter struct {
	Prefix     string // action prefix, e.g. "impersonation."
	MerchantID string // entity_id of the tenant (impersonation rows use entity_type=merchant)
	ActorID    string
	Limit      int
	BeforeID   string // keyset: only rows older than this id's created_at
}

// ListAudit reads audit_logs (read-only) for the DEWA panel.
func (s *Service) ListAudit(ctx context.Context, f AuditFilter) ([]AuditRow, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, a.actor_id::text, u.email, a.action, a.entity_type, a.entity_id, a.metadata, a.request_id, a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE ($1 = '' OR a.action LIKE $1||'%')
		  AND ($2 = '' OR (a.entity_type = 'merchant' AND a.entity_id = $2) OR a.metadata->>'merchant_id' = $2)
		  AND ($3 = '' OR a.actor_id = NULLIF($3,'')::uuid)
		  AND ($4 = '' OR (a.created_at, a.id) < (SELECT created_at, id FROM audit_logs WHERE id = NULLIF($4,'')::uuid))
		ORDER BY a.created_at DESC, a.id DESC LIMIT $5`,
		escapeLike(f.Prefix), f.MerchantID, f.ActorID, f.BeforeID, f.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[AuditRow])
}

// RecordImpersonated writes one audit row for a request made through an impersonation
// session. actor = the real DEWA, never the tenant.
func (s *Service) RecordImpersonated(ctx context.Context, p *Principal, requestID string, md map[string]any) error {
	if md == nil {
		md = map[string]any{}
	}
	md["session_id"] = p.SessionID
	md["merchant_id"] = p.MerchantID
	return audit.Record(ctx, s.db, audit.Entry{ActorID: p.UserID, Action: "impersonation.request",
		EntityType: "merchant", EntityID: p.MerchantID, RequestID: requestID, Metadata: md})
}
