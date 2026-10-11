-- +goose Up
-- API keys (prefix public, secret shown once, only its hash stored) and idempotency records.

CREATE TABLE api_keys (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id    uuid NOT NULL REFERENCES merchants(id),
    name           text NOT NULL,
    environment    text NOT NULL CHECK (environment IN ('sandbox','production')),
    prefix         text NOT NULL UNIQUE,            -- public lookup part of the key
    secret_hash    bytea NOT NULL,                  -- sha256(secret); secret is 256-bit random
    scopes         text[] NOT NULL CHECK (cardinality(scopes) > 0),
    created_by     uuid REFERENCES users(id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz,
    revoked_at     timestamptz,
    revoked_reason text,
    last_used_at   timestamptz,
    rotated_from   uuid REFERENCES api_keys(id)
);
CREATE INDEX api_keys_merchant_idx ON api_keys (merchant_id, created_at DESC);

-- Idempotency is enforced by the UNIQUE constraint, never by Redis/locks alone (PRD §0 #4).
CREATE TABLE idempotency_records (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     uuid NOT NULL REFERENCES merchants(id),
    operation       text NOT NULL,                  -- e.g. 'payment.create'
    key             text NOT NULL,
    request_hash    bytea NOT NULL,                 -- sha256 of canonical request
    status          text NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress','completed')),
    resource_type   text,
    resource_id     text,
    response_status integer,
    response_body   text,                          -- stored verbatim so replays are byte-identical
    created_at      timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz,
    expires_at      timestamptz NOT NULL,
    UNIQUE (merchant_id, operation, key)
);

-- +goose Down
DROP TABLE idempotency_records;
DROP TABLE api_keys;
