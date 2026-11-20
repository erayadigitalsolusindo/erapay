-- +goose Up
-- Top-up saldo QRISGate lewat pembayaran QRIS EraPay (tahap 2). Pembayarannya baris payments biasa dengan
-- purpose = 'qrisgate_topup': saat sukses, jurnalnya mengkredit qrisgate_credit (bukan merchant_payable),
-- tidak memicu webhook ke merchant, dan tidak ikut laporan/saldo transaksi merchant.
ALTER TABLE payments ADD COLUMN purpose text NOT NULL DEFAULT 'payment' CHECK (purpose IN ('payment','qrisgate_topup'));

-- +goose StatementBegin
DO $$
DECLARE c text;
BEGIN
    SELECT conname INTO c FROM pg_constraint
    WHERE conrelid = 'qrisgate_topups'::regclass AND contype = 'c' AND pg_get_constraintdef(oid) LIKE '%method%';
    EXECUTE format('ALTER TABLE qrisgate_topups DROP CONSTRAINT %I', c);
END $$;
-- +goose StatementEnd
ALTER TABLE qrisgate_topups ADD CONSTRAINT qrisgate_topups_method_check CHECK (method IN ('manual','qris'));
ALTER TABLE qrisgate_topups ADD COLUMN payment_id uuid UNIQUE REFERENCES payments(id);

-- +goose Down
-- Gagal bila sudah ada top-up QRIS: riwayat itu tidak boleh dibuang diam-diam.
ALTER TABLE qrisgate_topups DROP COLUMN payment_id;
ALTER TABLE qrisgate_topups DROP CONSTRAINT qrisgate_topups_method_check;
ALTER TABLE qrisgate_topups ADD CONSTRAINT qrisgate_topups_method_check CHECK (method IN ('manual'));
ALTER TABLE payments DROP COLUMN purpose;
