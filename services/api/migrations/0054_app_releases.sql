-- +goose Up
-- Rilis APK yang diunggah DEWA untuk diunduh tenant (saat ini: listener notifikasi QRISGate). Berkas disimpan di
-- disk (ERAPAY_APK_STORAGE_DIR); tabel ini hanya metadata + sha256. Rilis terbaru = yang diunduh; tidak ada hapus.
CREATE TABLE app_releases (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind         text NOT NULL CHECK (kind IN ('qrisgate_listener')),
    version_name text NOT NULL CHECK (version_name ~ '^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$'),
    sha256       text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
    storage_key  text NOT NULL,
    note         text CHECK (char_length(note) <= 500),
    uploaded_by  uuid REFERENCES users(id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, version_name)
);
CREATE INDEX app_releases_latest_idx ON app_releases (kind, created_at DESC);

CREATE TRIGGER app_releases_append_only BEFORE UPDATE OR DELETE ON app_releases
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- +goose Down
DROP TABLE app_releases;
