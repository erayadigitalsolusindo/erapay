-- +goose Up
-- Berkas KYC dienkripsi saat disimpan (AES-256-GCM, kunci ERAPAY_ENCRYPTION_KEY) dan punya checksum SHA-256 isi asli.
-- enc_v: 0 = berkas lama (belum terenkripsi, dibaca apa adanya sampai dimigrasi), 1 = AES-256-GCM.
-- Berkas lama dienkripsi ulang dengan: bootstrap encrypt-kyc-files (idempoten).
ALTER TABLE merchant_documents
    ADD COLUMN sha256 text,
    ADD COLUMN enc_v  smallint NOT NULL DEFAULT 0;
ALTER TABLE merchant_brand_evidences
    ADD COLUMN sha256 text,
    ADD COLUMN enc_v  smallint NOT NULL DEFAULT 0;
ALTER TABLE merchant_brands
    ADD COLUMN logo_sha256 text,
    ADD COLUMN logo_enc_v  smallint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE merchant_brands DROP COLUMN logo_enc_v, DROP COLUMN logo_sha256;
ALTER TABLE merchant_brand_evidences DROP COLUMN enc_v, DROP COLUMN sha256;
ALTER TABLE merchant_documents DROP COLUMN enc_v, DROP COLUMN sha256;
