package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
	"github.com/erayadigitalsolusindo/erapay/services/api/internal/auth"
)

type ARUSTenantInput struct {
	TenantID      uuid.UUID
	BusinessName  string
	OwnerName     string
	OwnerEmail    string
	OwnerPhone    string
	AcceptedTerms bool
	RequestID     string
}

type ARUSTenantLink struct {
	Linked         bool      `json:"linked"`
	TenantID       uuid.UUID `json:"tenant_id"`
	MerchantID     uuid.UUID `json:"merchant_id"`
	MerchantStatus string    `json:"merchant_status"`
	OwnerRole      string    `json:"owner_role"`
	Created        bool      `json:"created,omitempty"`
}

type ownerSetupNotifier interface {
	SendOwnerSetup(ctx context.Context, userID, email string) error
}

func (s *Service) ARUSTenantStatus(ctx context.Context, tenantID uuid.UUID) (ARUSTenantLink, error) {
	var link ARUSTenantLink
	err := s.db.QueryRow(ctx, `
		SELECT l.source_tenant_id, l.merchant_id, m.onboarding_status, COALESCE(r.code, '')
		FROM merchant_external_links l
		JOIN merchants m ON m.id = l.merchant_id
		LEFT JOIN merchant_users mu ON mu.merchant_id = l.merchant_id AND mu.user_id = l.owner_user_id
		LEFT JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
		WHERE l.source = 'arus' AND l.source_tenant_id = $1`, tenantID).Scan(
		&link.TenantID, &link.MerchantID, &link.MerchantStatus, &link.OwnerRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return ARUSTenantLink{}, ErrNotFound
	}
	if err != nil {
		return ARUSTenantLink{}, err
	}
	link.Linked = true
	return link, nil
}

func (s *Service) ProvisionARUSTenant(ctx context.Context, in ARUSTenantInput) (ARUSTenantLink, error) {
	in.BusinessName = strings.TrimSpace(in.BusinessName)
	in.OwnerName = strings.TrimSpace(in.OwnerName)
	in.OwnerEmail = strings.ToLower(strings.TrimSpace(in.OwnerEmail))
	in.OwnerPhone = strings.TrimSpace(in.OwnerPhone)
	if in.TenantID == uuid.Nil || utf8.RuneCountInString(in.BusinessName) < 3 || utf8.RuneCountInString(in.BusinessName) > 120 ||
		utf8.RuneCountInString(in.OwnerName) < 3 || utf8.RuneCountInString(in.OwnerName) > 100 || !in.AcceptedTerms {
		return ARUSTenantLink{}, &ValidationError{Msg: "tenant, business, owner, and terms acceptance are required"}
	}
	address, err := mail.ParseAddress(in.OwnerEmail)
	if err != nil || address.Address != in.OwnerEmail || len(in.OwnerEmail) > 254 {
		return ARUSTenantLink{}, &ValidationError{Msg: "owner email is invalid"}
	}
	if in.OwnerPhone != "" && (len(in.OwnerPhone) < 8 || len(in.OwnerPhone) > 16) {
		return ARUSTenantLink{}, &ValidationError{Msg: "owner phone is invalid"}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ARUSTenantLink{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('arus:' || $1::text, 0))`, in.TenantID); err != nil {
		return ARUSTenantLink{}, err
	}
	var link ARUSTenantLink
	var linkedOwnerID uuid.UUID
	linkErr := tx.QueryRow(ctx, `SELECT l.source_tenant_id, l.merchant_id, l.owner_user_id, m.onboarding_status, COALESCE(r.code, '')
		FROM merchant_external_links l
		JOIN merchants m ON m.id = l.merchant_id
		LEFT JOIN merchant_users mu ON mu.merchant_id = l.merchant_id AND mu.user_id = l.owner_user_id
		LEFT JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
		WHERE l.source = 'arus' AND l.source_tenant_id = $1`, in.TenantID).Scan(
		&link.TenantID, &link.MerchantID, &linkedOwnerID, &link.MerchantStatus, &link.OwnerRole)
	if linkErr == nil {
		if link.OwnerRole == "" {
			if _, err := tx.Exec(ctx, `INSERT INTO merchant_users (merchant_id, user_id, role_id)
				SELECT $1, $2, id FROM roles WHERE code = 'merchant_owner'
				ON CONFLICT (merchant_id, user_id) DO UPDATE SET role_id = EXCLUDED.role_id`, link.MerchantID, linkedOwnerID); err != nil {
				return ARUSTenantLink{}, err
			}
			link.OwnerRole = "merchant_owner"
		}
		link.Linked = true
		return link, tx.Commit(ctx)
	}
	if !errors.Is(linkErr, pgx.ErrNoRows) {
		return ARUSTenantLink{}, linkErr
	}

	type existingMerchant struct {
		merchantID uuid.UUID
		ownerID    uuid.UUID
		status     string
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT m.id, u.id, m.onboarding_status
		FROM merchants m
		JOIN merchant_users mu ON mu.merchant_id = m.id
		JOIN users u ON u.id = mu.user_id AND lower(u.email) = $1 AND u.status = 'active'
		JOIN roles r ON r.id = mu.role_id AND r.code = 'merchant_owner'
		WHERE (lower(trim(m.display_name)) = lower(trim($2)) OR lower(trim(m.legal_name)) = lower(trim($2)))
		AND NOT EXISTS (SELECT 1 FROM merchant_external_links l WHERE l.merchant_id = m.id)
		ORDER BY m.id LIMIT 2`, in.OwnerEmail, in.BusinessName)
	if err != nil {
		return ARUSTenantLink{}, err
	}
	var matches []existingMerchant
	for rows.Next() {
		var match existingMerchant
		if err := rows.Scan(&match.merchantID, &match.ownerID, &match.status); err != nil {
			rows.Close()
			return ARUSTenantLink{}, err
		}
		matches = append(matches, match)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ARUSTenantLink{}, err
	}
	if len(matches) > 1 {
		return ARUSTenantLink{}, ErrARUSMerchantAmbiguous
	}
	if len(matches) == 1 {
		match := matches[0]
		if _, err := tx.Exec(ctx, `INSERT INTO merchant_external_links (source, source_tenant_id, merchant_id, owner_user_id, terms_accepted_at, terms_accepted_by)
			VALUES ('arus', $1, $2, $3, now(), $4)`, in.TenantID, match.merchantID, match.ownerID, in.OwnerEmail); err != nil {
			return ARUSTenantLink{}, err
		}
		if err := audit.Record(ctx, tx, audit.Entry{ActorID: match.ownerID.String(), Action: "merchant.linked", EntityType: "merchant",
			EntityID: match.merchantID.String(), RequestID: in.RequestID,
			Metadata: map[string]any{"source": "arus", "source_tenant_id": in.TenantID.String(), "terms_accepted": true}}); err != nil {
			return ARUSTenantLink{}, err
		}
		link = ARUSTenantLink{Linked: true, TenantID: in.TenantID, MerchantID: match.merchantID,
			MerchantStatus: match.status, OwnerRole: "merchant_owner"}
		return link, tx.Commit(ctx)
	}

	var ownerID uuid.UUID
	var ownerStatus string
	var setupNotifier ownerSetupNotifier
	err = tx.QueryRow(ctx, `SELECT id, status FROM users WHERE lower(email) = $1 FOR UPDATE`, in.OwnerEmail).Scan(&ownerID, &ownerStatus)
	createdOwner := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !createdOwner {
		return ARUSTenantLink{}, err
	}
	if createdOwner {
		if s.notify == nil {
			return ARUSTenantLink{}, ErrEmailDeliveryUnavailable
		}
		var ok bool
		setupNotifier, ok = s.notify.(ownerSetupNotifier)
		if !ok {
			return ARUSTenantLink{}, ErrEmailDeliveryUnavailable
		}
		var rawPassword [32]byte
		if _, err := rand.Read(rawPassword[:]); err != nil {
			return ARUSTenantLink{}, err
		}
		passwordHash, err := auth.HashPassword(base64.RawURLEncoding.EncodeToString(rawPassword[:]))
		if err != nil {
			return ARUSTenantLink{}, err
		}
		err = tx.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, $2)
			ON CONFLICT DO NOTHING RETURNING id`, in.OwnerEmail, passwordHash).Scan(&ownerID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `SELECT id, status FROM users WHERE lower(email) = $1 FOR UPDATE`, in.OwnerEmail).Scan(&ownerID, &ownerStatus)
			createdOwner = false
		}
		if err != nil {
			return ARUSTenantLink{}, err
		}
	}
	if !createdOwner && ownerStatus != "active" {
		return ARUSTenantLink{}, ErrState
	}

	err = tx.QueryRow(ctx, `INSERT INTO merchants (legal_name, display_name, onboarding_status, account_type, owner_name, contact_phone)
		VALUES ($1, $1, 'submitted', 'business', $2, NULLIF($3, '')) RETURNING id`,
		in.BusinessName, in.OwnerName, in.OwnerPhone).Scan(&link.MerchantID)
	if err != nil {
		return ARUSTenantLink{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_outlets (merchant_id, name) VALUES ($1, 'Outlet Utama')`, link.MerchantID); err != nil {
		return ARUSTenantLink{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_users (merchant_id, user_id, role_id)
		SELECT $1, $2, id FROM roles WHERE code = 'merchant_owner'`, link.MerchantID, ownerID); err != nil {
		return ARUSTenantLink{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_external_links (source, source_tenant_id, merchant_id, owner_user_id, terms_accepted_at, terms_accepted_by)
		VALUES ('arus', $1, $2, $3, now(), $4)`, in.TenantID, link.MerchantID, ownerID, in.OwnerEmail); err != nil {
		return ARUSTenantLink{}, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: ownerID.String(), Action: "merchant.provisioned", EntityType: "merchant",
		EntityID: link.MerchantID.String(), RequestID: in.RequestID,
		Metadata: map[string]any{"source": "arus", "source_tenant_id": in.TenantID.String(), "terms_accepted": true}}); err != nil {
		return ARUSTenantLink{}, err
	}
	link.TenantID, link.MerchantStatus, link.OwnerRole, link.Linked, link.Created = in.TenantID, "submitted", "merchant_owner", true, true
	if err := tx.Commit(ctx); err != nil {
		return ARUSTenantLink{}, err
	}
	if createdOwner {
		if err := setupNotifier.SendOwnerSetup(ctx, ownerID.String(), in.OwnerEmail); err != nil {
			s.cfg.Log.Error("EraPay owner setup email could not be queued", "merchant_id", link.MerchantID, "err", err)
		}
	}
	return link, nil
}
