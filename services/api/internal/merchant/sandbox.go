package merchant

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/erayadigitalsolusindo/erapay/services/api/internal/audit"
)

// ErrNotApproved: only an approved production merchant can open a sandbox.
var ErrNotApproved = errors.New("merchant: must be approved (and not a sandbox) to enable sandbox")

// SetFakeGateway turns on the fake sandbox gateway for twins: each gets its own QRIS plan (priced like the
// merchant's DOKU plan, so sandbox fees look real) and an active account, the next time sandbox is enabled
// (or enabled again) for it. Off by default (docs/prd/gateway-new.md: flag default OFF).
func (r *Repo) SetFakeGateway(on bool) { r.fake = on }

// ensureFake gives the twin its fake-gateway plan and account, once. Safe to repeat.
func (r *Repo) ensureFake(ctx context.Context, tx pgx.Tx, parentID, twinID string) error {
	if !r.fake {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		WITH src AS (
		    SELECT p.* FROM merchant_pricing_assignments a JOIN pricing_plans p ON p.id = a.plan_id
		    WHERE a.merchant_id::text = $1 AND p.gateway = 'doku' AND p.payment_method = 'QRIS'
		    ORDER BY a.effective_from DESC LIMIT 1),
		new_plan AS (
		    INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_fixed, mdr_rounding, platform_bps, platform_fixed,
		                               platform_rounding, mdr_cap, mdr_cap_up_to, mdr_flat_up_to, mdr_flat_fee, created_by, approved_by)
		    SELECT 'fake-qris-' || $2::text, 1, 'fake', payment_method, mdr_bps, mdr_fixed, mdr_rounding, platform_bps, platform_fixed,
		           platform_rounding, mdr_cap, mdr_cap_up_to, mdr_flat_up_to, mdr_flat_fee, created_by, approved_by
		    FROM src WHERE NOT EXISTS (SELECT 1 FROM pricing_plans x WHERE x.code = 'fake-qris-' || $2::text)
		    RETURNING id)
		INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
		SELECT $2::uuid, id, now() FROM new_plan`, parentID, twinID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO merchant_gateway_accounts (merchant_id, gateway, environment, gateway_ref, activation_status)
		VALUES ($1, 'fake', 'sandbox', 'FAKE', 'active') ON CONFLICT (merchant_id, gateway, environment) DO NOTHING`, twinID)
	return err
}

// SandboxOf returns the sandbox twin of a merchant, or "" when it has none. Asked about a twin itself (a session
// switched to sandbox), it returns that twin.
func (r *Repo) SandboxOf(ctx context.Context, merchantID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id::text FROM merchants WHERE parent_id::text = $1 OR (id::text = $1 AND is_sandbox)`, merchantID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// EnableSandbox creates the merchant's sandbox twin (once; repeating returns the same one). The twin is
// approved without KYC, copies the pricing, has one outlet, and is deliberately NOT linked to any user:
// sessions pick a merchant by order, so a linked twin could silently become someone's default. It is
// reached through the parent's /v1/sandbox endpoints.
func (r *Repo) EnableSandbox(ctx context.Context, parentID, actorID, requestID string) (id string, created bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var status string
	var sbx bool
	if err := tx.QueryRow(ctx, `SELECT onboarding_status, is_sandbox FROM merchants WHERE id::text = $1 FOR UPDATE`, parentID).
		Scan(&status, &sbx); errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrNotFound
	} else if err != nil {
		return "", false, err
	}
	if sbx || status != "approved" {
		return "", false, ErrNotApproved
	}
	// The parent row lock serialises concurrent enables, so the twin is created exactly once.
	if err := tx.QueryRow(ctx, `SELECT id::text FROM merchants WHERE parent_id::text = $1`, parentID).Scan(&id); err == nil {
		// Already there; enabling again also brings a twin made before the fake gateway was switched on up to date.
		if err := r.ensureFake(ctx, tx, parentID, id); err != nil {
			return "", false, err
		}
		return id, false, tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO merchants (legal_name, display_name, category, onboarding_status, is_sandbox, parent_id)
		SELECT legal_name || ' (Sandbox)', display_name || ' (Sandbox)', category, 'approved', true, id
		FROM merchants WHERE id::text = $1 RETURNING id::text`, parentID).Scan(&id); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_outlets (merchant_id, name) VALUES ($1, 'Sandbox')`, id); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from)
		SELECT $1, plan_id, effective_from FROM merchant_pricing_assignments WHERE merchant_id::text = $2`, id, parentID); err != nil {
		return "", false, err
	}
	if err := r.ensureFake(ctx, tx, parentID, id); err != nil {
		return "", false, err
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorID: actorID, Action: "merchant.sandbox_enabled", EntityType: "merchant",
		EntityID: id, RequestID: requestID, Metadata: map[string]any{"parent_merchant_id": parentID, "sandbox_merchant_id": id}}); err != nil {
		return "", false, err
	}
	return id, true, tx.Commit(ctx)
}
