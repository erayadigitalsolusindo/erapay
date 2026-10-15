-- +goose Up
-- Impersonasi DEWA ("masuk sebagai tenant"): sesi merchant yang dibuka DEWA dengan alasan wajib.
-- Sesi ini tetap milik user DEWA (user_id), tetapi terikat ke satu merchant dan selalu diaudit.

ALTER TABLE sessions
    ADD COLUMN impersonator_id     uuid REFERENCES users(id),
    ADD COLUMN impersonation_reason text;

-- Hanya sesi impersonasi yang punya impersonator; reason wajib dan merchant wajib terisi.
ALTER TABLE sessions
    ADD CONSTRAINT sessions_impersonation_chk CHECK (
        impersonator_id IS NULL
        OR (merchant_id IS NOT NULL AND impersonator_id = user_id AND length(btrim(impersonation_reason)) >= 10)
    );

CREATE INDEX sessions_impersonator_idx ON sessions (impersonator_id) WHERE impersonator_id IS NOT NULL;

-- Penelusuran audit per tenant / per aktor.
CREATE INDEX audit_logs_action_idx ON audit_logs (action, created_at DESC);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_id, created_at DESC);

-- +goose Down
DROP INDEX audit_logs_actor_idx;
DROP INDEX audit_logs_action_idx;
DROP INDEX sessions_impersonator_idx;
ALTER TABLE sessions DROP CONSTRAINT sessions_impersonation_chk;
ALTER TABLE sessions DROP COLUMN impersonation_reason, DROP COLUMN impersonator_id;
