-- +goose Up
-- Aturan transaksi kecil (BI): transaksi <= mdr_flat_up_to dikenai MDR flat mdr_flat_fee (menggantikan
-- persen/nominal/cap). mdr_flat_up_to = 0 berarti aturan tidak dipakai (perilaku lama).
ALTER TABLE pricing_plans
    ADD COLUMN mdr_flat_up_to bigint NOT NULL DEFAULT 0 CHECK (mdr_flat_up_to >= 0),
    ADD COLUMN mdr_flat_fee   bigint NOT NULL DEFAULT 0 CHECK (mdr_flat_fee >= 0);

-- +goose Down
ALTER TABLE pricing_plans
    DROP COLUMN mdr_flat_fee,
    DROP COLUMN mdr_flat_up_to;
