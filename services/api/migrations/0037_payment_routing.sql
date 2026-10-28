-- +goose Up
-- Jalur pembayaran: gateway mana yang dipakai untuk transaksi BARU, diatur DEWA dari panel.
-- Satu baris per environment. Tanpa baris, perilaku lama berlaku (akun gateway aktif merchant yang
-- paling lama), jadi migrasi ini tidak mengubah jalur apa pun sampai DEWA memilih.
-- Transaksi yang sudah dibuat tetap memakai gateway yang mencatatnya (payments.gateway).
CREATE TABLE payment_routing (
    environment text PRIMARY KEY CHECK (environment IN ('sandbox','production')),
    gateway     text NOT NULL CHECK (char_length(gateway) BETWEEN 1 AND 16),
    version     integer NOT NULL DEFAULT 1 CHECK (version >= 0), -- 0 hanya sementara di dalam transaksi Set (baris belum pernah dipilih)
    updated_by  uuid REFERENCES users(id),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE payment_routing;
