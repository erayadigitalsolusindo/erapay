-- +goose Up
-- Nominal persis: pelanggan membayar tepat harga nota (tanpa kode unik). Bawaan mati (perilaku lama: harga + kode unik 1-99).
-- Hanya bila nominal yang sama sedang menunggu di toko yang sama, transaksi berikutnya tetap memakai kode unik supaya satu
-- notifikasi tidak bisa cocok dengan dua transaksi.
ALTER TABLE qrisgate_accounts ADD COLUMN exact_amount boolean NOT NULL DEFAULT false;
ALTER TABLE qrisgate_claims DROP CONSTRAINT qrisgate_claims_unique_code_check;
ALTER TABLE qrisgate_claims ADD CONSTRAINT qrisgate_claims_unique_code_check CHECK (unique_code BETWEEN 0 AND 99);

-- +goose Down
-- Gagal bila sudah ada klaim berkode 0: riwayat itu tidak dibuang diam-diam.
ALTER TABLE qrisgate_claims DROP CONSTRAINT qrisgate_claims_unique_code_check;
ALTER TABLE qrisgate_claims ADD CONSTRAINT qrisgate_claims_unique_code_check CHECK (unique_code BETWEEN 1 AND 99);
ALTER TABLE qrisgate_accounts DROP COLUMN exact_amount;
