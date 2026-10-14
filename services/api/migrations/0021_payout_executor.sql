-- +goose Up
-- Kirim DOKU payout executor (PRD payout.md, doku.md §6, decision 14 Okt 2026).
-- The executor drives an approved payout through the gateway: it claims it (approved ->
-- submitted), asks the gateway to move the funds, and later resolves the outcome via
-- check status. Non-breaking for existing rows: every new status leaves the old five
-- values valid, fee_amount defaults to 0 (no fee leg) and gateway stays NULL for
-- manually paid payouts.

ALTER TABLE payouts DROP CONSTRAINT payouts_status_check;
ALTER TABLE payouts ADD CONSTRAINT payouts_status_check CHECK (status IN
    ('pending_approval','approved','submitted','unknown','paid','failed','returned','cancelled'));

-- 'paid','failed','returned','cancelled' are final; 'unknown' (ambiguous gateway
-- outcome, PRD §0 #3) and 'submitted' stay resolvable by check status.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION payouts_immutable_fields() RETURNS trigger AS $$
BEGIN
    IF NEW.merchant_id <> OLD.merchant_id OR NEW.bank_account_id <> OLD.bank_account_id OR NEW.amount <> OLD.amount
       OR NEW.currency <> OLD.currency OR NEW.requested_by <> OLD.requested_by OR NEW.dry_run <> OLD.dry_run THEN
        RAISE EXCEPTION 'payout % money/destination fields are immutable', OLD.id;
    END IF;
    IF OLD.status IN ('paid','failed','returned','cancelled') AND NEW.status <> OLD.status THEN
        RAISE EXCEPTION 'payout % is final', OLD.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- gateway: which executor moved the funds ("doku" / "dry-run"); gateway_reference of the
-- transfer is the existing bank_reference. fee_amount is the transfer fee the merchant
-- bears (doku.md §6 decision): deducted from the nominal, booked as its own ledger leg.
ALTER TABLE payouts
    ADD COLUMN gateway    text CHECK (char_length(gateway) <= 16),
    ADD COLUMN fee_amount bigint NOT NULL DEFAULT 0 CHECK (fee_amount >= 0 AND fee_amount < amount);

CREATE INDEX payouts_executor_idx ON payouts (status, created_at) WHERE status IN ('submitted','unknown');

-- The transfer fee leg (doku.md §6): a platform liability for fees charged to merchants,
-- never merged into the cash that leaves.
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable'));

-- +goose Down
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable'));
DROP INDEX payouts_executor_idx;
ALTER TABLE payouts
    DROP COLUMN fee_amount,
    DROP COLUMN gateway;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION payouts_immutable_fields() RETURNS trigger AS $$
BEGIN
    IF NEW.merchant_id <> OLD.merchant_id OR NEW.bank_account_id <> OLD.bank_account_id OR NEW.amount <> OLD.amount
       OR NEW.currency <> OLD.currency OR NEW.requested_by <> OLD.requested_by OR NEW.dry_run <> OLD.dry_run THEN
        RAISE EXCEPTION 'payout % money/destination fields are immutable', OLD.id;
    END IF;
    IF OLD.status IN ('paid','failed','cancelled') AND NEW.status <> OLD.status THEN
        RAISE EXCEPTION 'payout % is final', OLD.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
ALTER TABLE payouts DROP CONSTRAINT payouts_status_check;
ALTER TABLE payouts ADD CONSTRAINT payouts_status_check CHECK (status IN ('pending_approval','approved','paid','failed','cancelled'));
