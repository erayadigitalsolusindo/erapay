-- +goose Up
-- QRISGate: gateway "langsung" (dana masuk ke rekening tenant sendiri, bukan lewat PT). Pembayarannya ditandai
-- direct: tidak ada clearing/settlement/refund dan tidak mengkredit merchant_payable; hanya potongan saldo kredit.
ALTER TABLE payments ADD COLUMN direct boolean NOT NULL DEFAULT false;

-- Harga jalur langsung: tanpa MDR dan tanpa Biaya Jasa persen (potongan per transaksi = setelan qrisgate_fee).
-- Dipakai langsung oleh CreateTx untuk gateway langsung, jadi tidak perlu assignment per merchant.
INSERT INTO pricing_plans (code, version, gateway, payment_method, mdr_bps, mdr_rounding, platform_bps, platform_rounding)
VALUES ('qrisgate-direct', 1, 'qrisgate', 'QRIS', 0, 'half_up', 0, 'half_up');

-- Kredensial QRISGate milik tiap tenant (API key + secret webhook), terenkripsi AES-GCM; write-only dari UI.
CREATE TABLE qrisgate_accounts (
    merchant_id        uuid PRIMARY KEY REFERENCES merchants(id),
    api_key_enc        bytea NOT NULL,
    webhook_secret_enc bytea NOT NULL,
    key_hint           text NOT NULL CHECK (char_length(key_hint) <= 16),
    updated_by         uuid REFERENCES users(id),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- Gagal bila sudah ada pembayaran langsung: riwayat itu tidak boleh dibuang diam-diam.
DROP TABLE qrisgate_accounts;
-- pricing_plans is append-only: the qrisgate-direct plan stays (inert without direct payments).
ALTER TABLE payments DROP COLUMN direct;
