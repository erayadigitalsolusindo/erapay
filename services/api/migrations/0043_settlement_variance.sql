-- +goose Up
-- Manual settlement may be booked with a variance (reported_net != expected_net) when Finance accepts it
-- explicitly with a written reason. Merchant balances still follow expected_net (gross - MDR at payment time);
-- the difference (cash actually received - expected) is booked to settlement_variance, i.e. PT absorbs it.
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_ids_clearing','cash_finpay_clearing','cash_xendit_clearing','cash_midtrans_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable','settlement_variance'));

-- +goose StatementBegin
DO $$
DECLARE c text;
BEGIN
    SELECT conname INTO c FROM pg_constraint
    WHERE conrelid = 'settlement_batches'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%reported_net = expected_net%';
    EXECUTE format('ALTER TABLE settlement_batches DROP CONSTRAINT %I', c);
END $$;
-- +goose StatementEnd
-- A mismatching batch needs a reason (note) on the row itself.
ALTER TABLE settlement_batches ADD CONSTRAINT settlement_batches_variance_note
    CHECK (reported_net = expected_net OR COALESCE(char_length(note), 0) >= 3);

-- +goose Down
-- Fails while a variance batch or settlement_variance account exists: that history must not be dropped silently.
ALTER TABLE settlement_batches DROP CONSTRAINT settlement_batches_variance_note;
ALTER TABLE settlement_batches ADD CONSTRAINT settlement_batches_check CHECK (reported_net = expected_net);
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_ids_clearing','cash_finpay_clearing','cash_xendit_clearing','cash_midtrans_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable'));
