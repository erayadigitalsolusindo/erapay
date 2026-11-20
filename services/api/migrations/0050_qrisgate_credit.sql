-- +goose Up
-- Saldo kredit QRISGate per tenant: top-up menambah, potongan per transaksi QRISGate mengurangi.
-- Akun ledger merchant-scoped qrisgate_credit (kewajiban EraPay ke tenant; saldo = kredit - debit, boleh minus).
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_ids_clearing','cash_finpay_clearing','cash_xendit_clearing','cash_midtrans_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable','settlement_variance','qrisgate_credit'));

-- +goose StatementBegin
DO $$
DECLARE c text;
BEGIN
    SELECT conname INTO c FROM pg_constraint
    WHERE conrelid = 'ledger_accounts'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%merchant_id IS NOT NULL%';
    EXECUTE format('ALTER TABLE ledger_accounts DROP CONSTRAINT %I', c);
END $$;
-- +goose StatementEnd
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_merchant_scope
    CHECK ((code IN ('merchant_payable','payout_in_transit','refund_payable','qrisgate_credit')) = (merchant_id IS NOT NULL));

-- Permintaan top-up manual: tenant minta, Finance konfirmasi setelah transfer masuk (pembuat <> penyetuju).
-- Saldo hanya bertambah lewat jurnal ledger saat dikonfirmasi; baris ini tidak pernah menjadi sumber saldo.
CREATE TABLE qrisgate_topups (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     uuid NOT NULL REFERENCES merchants(id),
    amount          bigint NOT NULL CHECK (amount > 0),
    currency        text NOT NULL,
    method          text NOT NULL DEFAULT 'manual' CHECK (method IN ('manual')),
    status          text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested','confirmed','rejected','cancelled')),
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 128),
    requested_by    uuid NOT NULL REFERENCES users(id),
    decided_by      uuid REFERENCES users(id),
    decided_at      timestamptz,
    bank_reference  text CHECK (char_length(bank_reference) <= 128),
    note            text CHECK (char_length(note) <= 500),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, idempotency_key),
    CHECK ((status = 'requested') = (decided_at IS NULL)),
    CHECK (status NOT IN ('confirmed','rejected') OR decided_by IS DISTINCT FROM requested_by)
);
CREATE INDEX qrisgate_topups_merchant_idx ON qrisgate_topups (merchant_id, created_at DESC);
CREATE INDEX qrisgate_topups_open_idx ON qrisgate_topups (created_at) WHERE status = 'requested';

-- +goose Down
-- Gagal bila akun qrisgate_credit sudah ada: riwayat itu tidak boleh dibuang diam-diam.
DROP TABLE qrisgate_topups;
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_merchant_scope;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_check
    CHECK ((code IN ('merchant_payable','payout_in_transit','refund_payable')) = (merchant_id IS NOT NULL));
ALTER TABLE ledger_accounts DROP CONSTRAINT ledger_accounts_code_check;
ALTER TABLE ledger_accounts ADD CONSTRAINT ledger_accounts_code_check CHECK (code IN (
    'cash_doku_clearing','cash_ids_clearing','cash_finpay_clearing','cash_xendit_clearing','cash_midtrans_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
    'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable','payout_fee_payable','settlement_variance'));
