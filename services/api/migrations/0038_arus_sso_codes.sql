-- +goose Up
-- Kode masuk-sekali dari ARUS ("Masuk ke EraPay" tanpa login ulang). Hanya sha256 yang disimpan,
-- berlaku 60 detik, dan hanya bisa ditukar sekali (used_at).
CREATE TABLE sso_codes (
    code_hash   bytea PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users(id),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz
);
CREATE INDEX sso_codes_expires_idx ON sso_codes (expires_at);

-- +goose Down
DROP TABLE sso_codes;
