-- +goose Up
-- Jalur pembayaran per tenant: menimpa pilihan platform (payment_routing) untuk satu merchant, mis. default platform DANA
-- tetapi tenant A ke QRISGate. Tanpa baris = ikut perilaku lama (pilihan platform, lalu akun aktif merchant).
-- fallback=false (bawaan): bila gateway pilihan tak bisa melayani merchant saat itu, pembayaran DITOLAK; QRISGate itu
-- uang langsung ke rekening tenant, jadi tidak boleh diam-diam pindah jalur. fallback=true: jatuh ke jalur biasa.
-- Pembayaran yang sudah dibuat tetap memakai gateway yang mencatatnya (payments.gateway).
CREATE TABLE merchant_payment_routing (
    merchant_id uuid NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    environment text NOT NULL CHECK (environment IN ('sandbox','production')),
    gateway     text NOT NULL CHECK (char_length(gateway) BETWEEN 1 AND 16),
    fallback    boolean NOT NULL DEFAULT false,
    version     integer NOT NULL DEFAULT 1 CHECK (version >= 0), -- 0 hanya sementara di dalam transaksi Set
    updated_by  uuid REFERENCES users(id),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (merchant_id, environment)
);

-- +goose Down
DROP TABLE merchant_payment_routing;
