-- +goose Up
-- Kredensial gateway tingkat platform (satu set per gateway + environment), dikelola DEWA lewat panel.
-- Isi (JSON field->nilai) disegel AES-256-GCM dengan ERAPAY_ENCRYPTION_KEY, AAD = gwcred/<gateway>/<env>.
-- version dipakai untuk optimistic locking: dua admin tidak saling menimpa diam-diam.
-- Tanpa baris di sini, server tetap memakai env DOKU_* seperti sebelumnya.
CREATE TABLE gateway_credentials (
    gateway     text        NOT NULL,
    environment text        NOT NULL CHECK (environment IN ('sandbox', 'production')),
    sealed      bytea       NOT NULL,
    version     integer     NOT NULL DEFAULT 0,
    updated_by  uuid        REFERENCES users(id),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (gateway, environment)
);

-- +goose Down
DROP TABLE gateway_credentials;
