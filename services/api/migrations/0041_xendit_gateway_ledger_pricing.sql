-- +goose Up
-- Xendit as a fourth QRIS gateway. Its funds sit with Xendit, so the expected-from-gateway balance
-- gets its own clearing account (cash_xendit_clearing): DOKU and IDS books stay as they were and
-- each gateway's clearing reconciles on its own.
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_ids_clearing','cash_finpay_clearing','cash_xendit_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable'));

-- A payment is priced by the plan of the gateway that routes it. Give every merchant that already
-- has a current DOKU QRIS plan a Xendit plan with the same fees (same rule as FINPAY, owner decision
-- 18 Nov 2026; Xendit's real MDR is unknown and any difference surfaces as a settlement mismatch). Merchants approved
-- from now on get it at approval (onboarding.PlanGateways). Re-running is safe: a merchant that
-- already has a xendit- plan is skipped.
WITH latest AS (
    SELECT DISTINCT ON (a.merchant_id) a.merchant_id, p.*
    FROM merchant_pricing_assignments a JOIN pricing_plans p ON p.id = a.plan_id
    WHERE p.gateway = 'doku' AND p.payment_method = 'QRIS'
    ORDER BY a.merchant_id, a.effective_from DESC
), new_plans AS (
    INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_fixed, mdr_rounding,
                               platform_bps, platform_fixed, platform_rounding, mdr_cap, mdr_cap_up_to,
                               mdr_flat_up_to, mdr_flat_fee, created_by, approved_by)
    SELECT 'xendit-' || l.code, 1, 'xendit', l.payment_method, l.mdr_bps, l.mdr_fixed, l.mdr_rounding,
           l.platform_bps, l.platform_fixed, l.platform_rounding, l.mdr_cap, l.mdr_cap_up_to,
           l.mdr_flat_up_to, l.mdr_flat_fee, l.created_by, l.approved_by
    FROM latest l
    WHERE NOT EXISTS (SELECT 1 FROM pricing_plans x WHERE x.code = 'xendit-' || l.code)
    RETURNING id, code
)
INSERT INTO merchant_pricing_assignments (merchant_id, plan_id, effective_from, created_by)
SELECT l.merchant_id, n.id, now(), l.created_by
FROM new_plans n JOIN latest l ON n.code = 'xendit-' || l.code;

-- +goose Down
-- pricing_plans is append-only, so the copied xendit plans stay; they are inert without a xendit account.
-- Fails if a ledger account cash_xendit_clearing exists: that history must not be dropped silently.
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_ids_clearing','cash_finpay_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable'));
