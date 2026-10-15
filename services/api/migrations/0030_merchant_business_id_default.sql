-- +goose Up
-- Business ID dibuat otomatis oleh database untuk setiap merchant baru (sebelumnya hanya
-- di-backfill sekali di 0023, sehingga merchant yang mendaftar sesudahnya kosong).
ALTER TABLE merchants
    ALTER COLUMN business_id SET DEFAULT 'BIZ-' || upper(substring(replace(gen_random_uuid()::text,'-','') from 1 for 8));

UPDATE merchants SET business_id = 'BIZ-' || upper(substring(replace(gen_random_uuid()::text,'-','') from 1 for 8))
WHERE business_id IS NULL OR business_id = '';

-- +goose Down
ALTER TABLE merchants ALTER COLUMN business_id DROP DEFAULT;
