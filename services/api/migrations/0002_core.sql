-- +goose Up

-- ============ Identity & tenancy (PRD §4.1, §4.2, §2) ============
CREATE TABLE merchants (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code        text NOT NULL UNIQUE,
    legal_name  text NOT NULL,
    -- Status onboarding EraPay. Status per gateway disimpan terpisah (gateway_merchant_mappings).
    status      text NOT NULL DEFAULT 'draft'
                CHECK (status IN ('draft','submitted','under_review','action_required','approved','rejected','suspended')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    version     integer NOT NULL DEFAULT 1
);

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL,
    name          text NOT NULL,
    password_hash text NOT NULL,           -- argon2id; bukan password asli
    mfa_secret_enc bytea,                  -- terenkripsi; null = MFA belum aktif
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

CREATE TABLE roles (
    id          text PRIMARY KEY,
    description text NOT NULL
);
INSERT INTO roles (id, description) VALUES
    ('super_admin',    'Konfigurasi global, user/role, gateway'),
    ('ops',            'Cari merchant/transaksi, timeline, catatan'),
    ('finance',        'Laporan, rekonsiliasi, payout (maker-checker)'),
    ('merchant_owner', 'Pemilik merchant'),
    ('cashier',        'Kasir outlet'),
    ('integrator',     'Integrator POS');

-- merchant_id NULL = peran platform (admin/ops/finance); terisi = peran di dalam satu merchant.
CREATE TABLE user_roles (
    user_id     uuid NOT NULL REFERENCES users(id),
    role_id     text NOT NULL REFERENCES roles(id),
    merchant_id uuid REFERENCES merchants(id),
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX user_roles_unique ON user_roles (user_id, role_id, COALESCE(merchant_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX user_roles_merchant_idx ON user_roles (merchant_id);

CREATE TABLE api_keys (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    environment text NOT NULL CHECK (environment IN ('sandbox','production')),
    prefix      text NOT NULL UNIQUE,      -- bagian publik kunci, untuk lookup
    secret_hash bytea NOT NULL,            -- hash; secret asli tampil sekali saja
    scopes      text[] NOT NULL DEFAULT '{}',
    created_by  uuid REFERENCES users(id),
    last_used_at timestamptz,
    revoked_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX api_keys_merchant_idx ON api_keys (merchant_id);

-- Satu gateway per (merchant, metode bayar). Saat ini hanya 'doku'.
CREATE TABLE gateway_merchant_mappings (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id        uuid NOT NULL REFERENCES merchants(id),
    gateway            text NOT NULL,
    payment_method     text NOT NULL,
    gateway_merchant_ref text,
    activation_status  text NOT NULL DEFAULT 'pending_confirmation'
                       CHECK (activation_status IN ('pending_confirmation','active','suspended')),
    is_default         boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, gateway, payment_method)
);

-- ============ Idempotency (PRD §5, §0 #4) ============
-- Unique constraint PostgreSQL adalah pengaman utama; Redis hanya bantuan.
CREATE TABLE idempotency_records (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id   uuid NOT NULL REFERENCES merchants(id),
    operation     text NOT NULL,
    key           text NOT NULL,
    request_hash  bytea NOT NULL,          -- hash request kanonik
    status        text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress','completed')),
    resource_id   uuid,
    response_code integer,
    response_body jsonb,
    expires_at    timestamptz NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, operation, key)
);

-- ============ Payments (PRD §4.3, §6) ============
CREATE TABLE payments (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id        uuid NOT NULL REFERENCES merchants(id),
    merchant_reference text NOT NULL,
    outlet_code        text,
    gateway            text NOT NULL,
    payment_method     text NOT NULL,
    gateway_reference  text,
    gateway_raw_status text,               -- status mentah gateway, apa adanya
    status             text NOT NULL DEFAULT 'created'
                       CHECK (status IN ('created','pending','succeeded','failed','expired','cancelled','unknown')),
    currency           char(3) NOT NULL,
    -- Uang = integer satuan terkecil (PRD §0 #1). Snapshot fee immutable (PRD §0 #8).
    amount             bigint NOT NULL CHECK (amount > 0),   -- gross_amount
    fee_bearer         text NOT NULL DEFAULT 'merchant' CHECK (fee_bearer IN ('merchant','customer')),
    mdr_fee            bigint NOT NULL CHECK (mdr_fee >= 0),
    platform_fee       bigint NOT NULL CHECK (platform_fee >= 0),
    tax                bigint NOT NULL DEFAULT 0 CHECK (tax >= 0),  -- OPEN QUESTION: PPN atas Biaya Jasa
    merchant_net       bigint NOT NULL CHECK (merchant_net >= 0),
    expires_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    version            integer NOT NULL DEFAULT 1,
    -- gross harus tepat sama dengan jumlah komponennya; tak boleh ada selisih hilang.
    CONSTRAINT payments_fee_sums CHECK (amount = mdr_fee + platform_fee + tax + merchant_net)
);
CREATE UNIQUE INDEX payments_gateway_ref_key ON payments (gateway, gateway_reference) WHERE gateway_reference IS NOT NULL;
CREATE INDEX payments_merchant_created_idx ON payments (merchant_id, created_at DESC);
CREATE INDEX payments_merchant_ref_idx ON payments (merchant_id, merchant_reference);
CREATE INDEX payments_status_idx ON payments (status);

-- Snapshot nominal & fee tidak boleh berubah setelah dibuat; payment tidak boleh dihapus.
-- +goose StatementBegin
CREATE FUNCTION payments_guard() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'payments tidak boleh dihapus';
    END IF;
    IF NEW.merchant_id IS DISTINCT FROM OLD.merchant_id
       OR NEW.currency     IS DISTINCT FROM OLD.currency
       OR NEW.amount       IS DISTINCT FROM OLD.amount
       OR NEW.fee_bearer   IS DISTINCT FROM OLD.fee_bearer
       OR NEW.mdr_fee      IS DISTINCT FROM OLD.mdr_fee
       OR NEW.platform_fee IS DISTINCT FROM OLD.platform_fee
       OR NEW.tax          IS DISTINCT FROM OLD.tax
       OR NEW.merchant_net IS DISTINCT FROM OLD.merchant_net THEN
        RAISE EXCEPTION 'snapshot nominal/fee payment bersifat immutable';
    END IF;
    -- Status terminal tidak boleh dibalik (PRD §4.3): hanya 'unknown' dan 'pending' yang dapat berubah.
    IF OLD.status IN ('succeeded','failed','expired','cancelled') AND NEW.status IS DISTINCT FROM OLD.status THEN
        RAISE EXCEPTION 'status terminal % tidak boleh berubah', OLD.status;
    END IF;
    NEW.updated_at := now();
    NEW.version := OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER payments_guard_trg BEFORE UPDATE OR DELETE ON payments
    FOR EACH ROW EXECUTE FUNCTION payments_guard();

-- +goose Down
DROP TRIGGER payments_guard_trg ON payments;
DROP FUNCTION payments_guard();
DROP TABLE payments, idempotency_records, gateway_merchant_mappings, api_keys, user_roles, roles, users, merchants;
