-- +goose Up
-- DANA as a sixth QRIS gateway. Its funds sit with DANA, so the expected-from-gateway balance gets its own
-- clearing account (cash_dana_clearing), same as IDS, FINPAY, Xendit and Midtrans.
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_ids_clearing','cash_finpay_clearing','cash_xendit_clearing','cash_midtrans_clearing','cash_dana_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable','settlement_variance','qrisgate_credit'));

-- A payment is priced by the plan of the gateway that routes it. Give every merchant that already has a
-- current DOKU QRIS plan a DANA plan with the same fees (same rule as FINPAY/Xendit/Midtrans; DANA's real
-- MDR is unknown and any difference surfaces as a settlement mismatch). Merchants approved from now on get it
-- at approval (onboarding.PlanGateways). Re-running is safe: a (code, version) that already has a dana- plan is skipped.
-- Merchants share plans, so one DANA plan is created per (code, version) of the plans in use, keeping each
-- version's fees exactly (a plain INSERT ... SELECT per merchant collides on UNIQUE (code, version)).
WITH latest AS (
    SELECT DISTINCT ON (a.merchant_id) a.merchant_id, p.*
    FROM merchant_pricing_assignments a JOIN pricing_plans p ON p.id = a.plan_id
    WHERE p.gateway = 'doku' AND p.payment_method = 'QRIS'
    ORDER BY a.merchant_id, a.effective_from DESC
), used_plans AS (
    SELECT DISTINCT ON (l.code, l.version) l.* FROM latest l ORDER BY l.code, l.version
), new_plans AS (
    INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_fixed, mdr_rounding,
                               platform_bps, platform_fixed, platform_rounding, mdr_cap, mdr_cap_up_to,
                               mdr_flat_up_to, mdr_flat_fee, created_by, approved_by)
    SELECT 'dana-' || u.code, u.version, 'dana', u.payment_method, u.mdr_bps, u.mdr_fixed, u.mdr_rounding,
           u.platform_bps, u.platform_fixed, u.platform_rounding, u.mdr_cap, u.mdr_cap_up_to,
           u.mdr_flat_up_to, u.mdr_flat_fee, u.created_by, u.approved_by
    FROM used_plans u
    WHERE NOT EXISTS (SELECT 1 FROM pricing_plans x WHERE x.code = 'dana-' || u.code AND x.version = u.version)
    RETURNING id, code, version
)
INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from, created_by)
SELECT l.merchant_id, n.id, now(), l.created_by
FROM new_plans n JOIN latest l ON n.code = 'dana-' || l.code AND n.version = l.version;

-- +goose Down
-- pricing_plans is append-only, so the copied dana plans stay; they are inert without a dana account.
-- Fails if a ledger account cash_dana_clearing exists: that history must not be dropped silently.
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_ids_clearing','cash_finpay_clearing','cash_xendit_clearing','cash_midtrans_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable','settlement_variance','qrisgate_credit'));
