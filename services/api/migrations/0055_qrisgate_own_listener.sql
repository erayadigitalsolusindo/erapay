-- +goose Up
-- QRISGate buatan sendiri: bukan lagi klien ke layanan pihak ketiga. Tenant menyimpan QRIS statis miliknya; EraPay
-- membuat QRIS dinamis dengan nominal unik (harga + kode unik 1-99) dan mengenali pembayaran dari notifikasi
-- aplikasi bank/e-wallet yang diteruskan listener Android (apps/listener-android) ke webhook bertanda tangan.
-- Kunci/secret layanan pihak ketiga dibuang (belum ada tenant yang memakainya).
ALTER TABLE qrisgate_accounts DROP COLUMN api_key_enc;
ALTER TABLE qrisgate_accounts DROP COLUMN webhook_secret_enc;
ALTER TABLE qrisgate_accounts RENAME COLUMN key_hint TO secret_hint;
ALTER TABLE qrisgate_accounts ALTER COLUMN secret_hint DROP NOT NULL;
ALTER TABLE qrisgate_accounts
    ADD COLUMN static_qris         text CHECK (char_length(static_qris) <= 1024),
    ADD COLUMN qris_name           text,
    ADD COLUMN listener_secret_enc bytea,                       -- secret HMAC listener, AES-GCM, tampil sekali saat dibuat
    ADD COLUMN packages            text[] NOT NULL DEFAULT '{}'; -- daftar nama paket aplikasi yang notifikasinya diterima

-- Klaim nominal: satu nominal bayar (harga + kode unik) hanya boleh dipegang satu transaksi aktif per tenant,
-- sehingga notifikasi "Rp 25.007 masuk" cocok dengan paling banyak satu transaksi. Dilepas saat transaksi final/kedaluwarsa.
CREATE TABLE qrisgate_claims (
    payment_id  uuid PRIMARY KEY REFERENCES payments(id),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    pay_amount  bigint NOT NULL CHECK (pay_amount > 0),
    unique_code integer NOT NULL CHECK (unique_code BETWEEN 1 AND 99),
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    released_at timestamptz
);
CREATE UNIQUE INDEX qrisgate_claims_active_uq ON qrisgate_claims (merchant_id, pay_amount) WHERE released_at IS NULL;

-- Setiap notifikasi yang diterima (kunci idempotensi per tenant = delivery id). Catatan audit sekaligus daftar
-- "tidak cocok" untuk dicek tenant. Teks notifikasi bisa memuat nama pengirim: hanya tenant pemilik yang melihatnya.
CREATE TABLE qrisgate_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    delivery_id text NOT NULL CHECK (char_length(delivery_id) BETWEEN 1 AND 128),
    package     text NOT NULL CHECK (char_length(package) <= 200),
    title       text NOT NULL CHECK (char_length(title) <= 300),
    body        text NOT NULL CHECK (char_length(body) <= 1000),
    amount      bigint,
    posted_at   timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    status      text NOT NULL CHECK (status IN ('matched','unmatched','ignored')),
    reason      text,
    payment_id  uuid REFERENCES payments(id),
    UNIQUE (merchant_id, delivery_id)
);
CREATE INDEX qrisgate_events_merchant_idx ON qrisgate_events (merchant_id, received_at DESC);

-- +goose Down
-- Gagal bila sudah ada klaim/event: riwayat itu tidak boleh dibuang diam-diam.
DROP TABLE qrisgate_events;
DROP TABLE qrisgate_claims;
ALTER TABLE qrisgate_accounts DROP COLUMN packages, DROP COLUMN listener_secret_enc, DROP COLUMN qris_name, DROP COLUMN static_qris;
ALTER TABLE qrisgate_accounts RENAME COLUMN secret_hint TO key_hint;
UPDATE qrisgate_accounts SET key_hint = '' WHERE key_hint IS NULL;
ALTER TABLE qrisgate_accounts ALTER COLUMN key_hint SET NOT NULL;
ALTER TABLE qrisgate_accounts ADD COLUMN api_key_enc bytea NOT NULL DEFAULT ''::bytea, ADD COLUMN webhook_secret_enc bytea NOT NULL DEFAULT ''::bytea;
