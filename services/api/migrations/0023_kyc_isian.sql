-- +goose Up
-- KYC Isian (data diri wajib gaya DOKU) untuk verifikasi merchant.
-- Status tetap SATU mekanisme: merchants.onboarding_status (0002) dipakai sebagai sumber kebenaran;
-- DRAFT=draft, SUBMITTED=submitted, IN_REVIEW=under_review, APPROVED=approved, REJECTED=rejected,
-- NEEDS_REVISION=action_required. NEEDS_REVIEW_AGAIN tidak jadi status baru: ditandai
-- kyc_submissions.recheck=true saat merchant APPROVED mengubah field sensitif
-- (kebijakan akses produksi dikonfigurasi lewat ERAPAY_KYC_RECHECK_POLICY, default tetap terbuka).
-- Transisi dicatat di audit_logs yang sudah append-only (PRD §0 #6) lewat paket audit,
-- jadi TIDAK ada tabel audit kedua.
-- IDEMPOTEN: semua DDL pakai IF NOT EXISTS agar aman di-re-run bila versi 23 gagal
-- sebagian (mis. di awal deploy) — goose menjalankan SQL apa adanya, tanpa transaksi.
-- Waktu dulu gagal: encode(uuid,…) tidak ada. Kini pakai replace(uuid::text,'-','').

ALTER TABLE merchants
    ADD COLUMN IF NOT EXISTS business_id     text UNIQUE, -- "BIZ-XXXXXXXX", dibuat server, tidak bisa diubah
    ADD COLUMN IF NOT EXISTS entity_type     text CHECK (entity_type IN ('PT','CV','Yayasan')),
    ADD COLUMN IF NOT EXISTS contact_email   text,
    ADD COLUMN IF NOT EXISTS addr_postal_code text,
    ADD COLUMN IF NOT EXISTS addr_kelurahan  text,
    ADD COLUMN IF NOT EXISTS addr_kecamatan  text,
    ADD COLUMN IF NOT EXISTS addr_kota       text,
    ADD COLUMN IF NOT EXISTS addr_provinsi   text,
    ADD COLUMN IF NOT EXISTS addr_detail     text,        -- maks 500 karakter
    ADD COLUMN IF NOT EXISTS owner_position  text CHECK (owner_position IN ('Pemilik','Direktur','Komisaris','Manajer')),
    ADD COLUMN IF NOT EXISTS owner_nationality text DEFAULT 'Indonesia',
    ADD COLUMN IF NOT EXISTS owner_phone     text,
    ADD COLUMN IF NOT EXISTS owner_email     text,
    ADD COLUMN IF NOT EXISTS business_kbli   text;         -- 5 digit KBLI (pasangan NIB)

-- Backfill Business ID untuk merchant lama & ARUS; yang baru dibuat oleh kyc.Service.
-- replace(gen_random_uuid()::text,'-','') = 32 hex tanpa tanda hubung; ambil 8 pertama.
UPDATE merchants SET business_id = 'BIZ-' || upper(substring(replace(gen_random_uuid()::text,'-','') from 1 for 8))
WHERE business_id IS NULL;

CREATE TABLE IF NOT EXISTS merchant_brands (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id    uuid NOT NULL UNIQUE REFERENCES merchants(id),
    brand_id       text NOT NULL UNIQUE,    -- "BRD-XXXXXXXX", dibuat server
    name           text,
    category       text,
    description    text,
    monthly_amount bigint NOT NULL DEFAULT 0, -- Rupiah, integer (PRD §0)
    monthly_count  bigint NOT NULL DEFAULT 0,
    logo_key       text,
    logo_mime      text,
    logo_name      text,
    logo_size      bigint,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS merchant_brand_evidences (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id      uuid NOT NULL REFERENCES merchant_brands(id) ON DELETE CASCADE,
    storage_key   text NOT NULL,
    original_name text NOT NULL,
    mime          text NOT NULL,
    size_bytes    bigint NOT NULL,
    uploaded_by   uuid REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS brand_evidences_brand_idx ON merchant_brand_evidences (brand_id);

CREATE TABLE IF NOT EXISTS merchant_brand_links (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id  uuid NOT NULL REFERENCES merchant_brands(id) ON DELETE CASCADE,
    url       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS merchant_brand_links_brand_idx ON merchant_brand_links (brand_id);

-- Dokumen KYC. Satu jenis satu file (unggah ulang menimpa); akses dibatasi pemilik + reviewer.
CREATE TABLE IF NOT EXISTS merchant_documents (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id   uuid NOT NULL REFERENCES merchants(id),
    doc_type      text NOT NULL CHECK (doc_type IN
        ('ktp','npwp_badan','akta','sk_kemenkumham','nib','izin_usaha')),
    storage_key   text NOT NULL,
    original_name text NOT NULL,
    mime          text NOT NULL,
    size_bytes    bigint NOT NULL,
    uploaded_by   uuid REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, doc_type)
);
CREATE INDEX IF NOT EXISTS merchant_documents_merchant_idx ON merchant_documents (merchant_id);

-- Satu baris per merchant: penanda proses KYC (status tetap di merchants.onboarding_status).
CREATE TABLE IF NOT EXISTS kyc_submissions (
    merchant_id  uuid PRIMARY KEY REFERENCES merchants(id),
    submitted_at timestamptz,
    reviewed_at  timestamptz,
    reviewer_id  uuid REFERENCES users(id),
    -- NEEDS_REVIEW_AGAIN: APPROVED + field sensitif berubah. Akses produksi tetap mengikuti policy.
    recheck      boolean NOT NULL DEFAULT false,
    recheck_sections jsonb NOT NULL DEFAULT '[]',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- Catatan reviewer per bagian/field (ditandai "perlu revisi" sampai merchant koreksi + submit ulang).
CREATE TABLE IF NOT EXISTS kyc_review_notes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    section     text NOT NULL CHECK (section IN ('company','representative','brand','documents')),
    field       text,                   -- kosong = catatan untuk seluruh bagian
    note        text NOT NULL,
    resolved    boolean NOT NULL DEFAULT false,
    reviewer_id uuid REFERENCES users(id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz
);
CREATE INDEX IF NOT EXISTS kyc_review_notes_merchant_idx ON kyc_review_notes (merchant_id, resolved);

-- Wilayah untuk autocomplete alamat (data contoh; produksi diganti data Pos Indonesia).
CREATE TABLE IF NOT EXISTS kyc_regions (
    id           bigserial PRIMARY KEY,
    postal_code  text NOT NULL,
    kelurahan    text NOT NULL,
    kecamatan    text NOT NULL,
    kota         text NOT NULL,
    provinsi     text NOT NULL
);
CREATE INDEX IF NOT EXISTS kyc_regions_search_idx ON kyc_regions (lower(kelurahan), lower(kota));

INSERT INTO kyc_regions (postal_code, kelurahan, kecamatan, kota, provinsi) VALUES
    ('12760','Duren Tiga','Pancoran','Jakarta Selatan','DKI Jakarta'),
    ('10110','Kebon Kelapa','Gambir','Jakarta Pusat','DKI Jakarta'),
    ('12190','Kebayoran Baru','Kebayoran Baru','Jakarta Selatan','DKI Jakarta'),
    ('13930','Pulo Gadung','Pulo Gadung','Jakarta Timur','DKI Jakarta'),
    ('14450','Cipondoh','Cipondoh','Kota Tangerang','Banten'),
    ('40111','Babakan Ciamis','Sumur Bandung','Kota Bandung','Jawa Barat'),
    ('40115','Cipedes','Coblong','Kota Bandung','Jawa Barat'),
    ('50132','Pandansari','Semarang Tengah','Kota Semarang','Jawa Tengah'),
    ('55165','Kotagede','Kotagede','Kota Yogyakarta','DI Yogyakarta'),
    ('60111','Ketabang','Genteng','Surabaya','Jawa Timur'),
    ('60281','Darmo','Wonokromo','Surabaya','Jawa Timur'),
    ('70114','Kertak Hanyar I','Banjarmasin Tengah','Banjarmasin','Kalimantan Selatan'),
    ('90111','Ujung Pandang','Ujung Pandang','Makassar','Sulawesi Selatan'),
    ('95127','Tumou Tou','Tumou Tou','Kota Manado','Sulawesi Utara'),
    ('10110','Glodok','Taman Sari','Jakarta Barat','DKI Jakarta'),
    ('15810','Cikokol','Tangerang','Kota Tangerang','Banten'),
    ('16911','Cibinong','Cibinong','Bogor','Jawa Barat'),
    ('80111','Dauh Puri Klod','Denpasar Barat','Denpasar','Bali'),
    ('23111','Keudah','Kuta Alam','Banda Aceh','Aceh'),
    ('28111','Sukajadi','Sukajadi','Pekanbaru','Riau'),
    ('20111','Petisah Hulu','Medan Baru','Medan','Sumatera Utara'),
    ('30111','Kedukan Bukit','Ilir Timur I','Palembang','Sumatera Selatan'),
    ('99112','Abepura','Abepura','Jayapura','Papua'),
    ('94112','Makassar','Makassar','Makassar','Sulawesi Selatan');

-- +goose Down
DROP TABLE IF EXISTS kyc_regions;
DROP TABLE IF EXISTS kyc_review_notes;
DROP TABLE IF EXISTS kyc_submissions;
DROP TABLE IF EXISTS merchant_documents;
DROP TABLE IF EXISTS merchant_brand_links;
DROP TABLE IF EXISTS merchant_brand_evidences;
DROP TABLE IF EXISTS merchant_brands;
ALTER TABLE merchants
    DROP COLUMN IF EXISTS business_kbli, DROP COLUMN IF EXISTS owner_email, DROP COLUMN IF EXISTS owner_phone,
    DROP COLUMN IF EXISTS owner_position, DROP COLUMN IF EXISTS owner_nationality, DROP COLUMN IF EXISTS addr_detail,
    DROP COLUMN IF EXISTS addr_kota, DROP COLUMN IF EXISTS addr_kecamatan, DROP COLUMN IF EXISTS addr_kelurahan,
    DROP COLUMN IF EXISTS addr_postal_code, DROP COLUMN IF EXISTS contact_email, DROP COLUMN IF EXISTS entity_type,
    DROP COLUMN IF EXISTS business_id;