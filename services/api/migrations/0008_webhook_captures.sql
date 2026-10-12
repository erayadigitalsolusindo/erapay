-- +goose Up
-- Raw inbound gateway notifications, stored VERBATIM for analysis only. Nothing reads these to
-- change a payment: the DOKU QRIS notification payload and verification token source are still
-- unconfirmed (docs/integrations/doku.md §4), so we capture real traffic before writing a parser.
-- The Authorization header is never stored; only its length and SHA-256 (enough to test a
-- hypothesis about where the token came from without keeping a bearer secret).

CREATE TABLE webhook_captures (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway        text NOT NULL,
    method         text NOT NULL,
    path           text NOT NULL,
    query          text NOT NULL DEFAULT '',
    headers        jsonb NOT NULL,                 -- Authorization removed
    auth_present   boolean NOT NULL,
    auth_len       integer,
    auth_sha256    text,
    body           bytea NOT NULL,
    body_sha256    text NOT NULL,
    body_truncated boolean NOT NULL DEFAULT false,
    remote_addr    text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webhook_captures_gateway_idx ON webhook_captures (gateway, created_at DESC);
CREATE TRIGGER webhook_captures_append_only BEFORE UPDATE OR DELETE ON webhook_captures
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- +goose Down
DROP TABLE webhook_captures;
