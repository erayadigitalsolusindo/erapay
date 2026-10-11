-- +goose Up
-- gen_random_uuid() is built in since PostgreSQL 13; no extension needed.

-- Append-only audit trail (PRD §0 #6): no UPDATE/DELETE allowed.
CREATE TABLE audit_logs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id    uuid,
    action      text NOT NULL,
    entity_type text NOT NULL,
    entity_id   text,
    metadata    jsonb NOT NULL DEFAULT '{}',
    request_id  text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose StatementBegin
CREATE FUNCTION forbid_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% on % is not allowed (append-only)', TG_OP, TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_logs_append_only
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- +goose Down
DROP TABLE audit_logs;
DROP FUNCTION forbid_mutation();
