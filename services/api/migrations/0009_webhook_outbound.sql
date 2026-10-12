-- +goose Up
-- Outbound webhooks to merchants (PRD webhook-outbound.md). Transactional outbox: the event and its
-- delivery rows are written in the SAME transaction as the payment transition, so a committed
-- status change can never lose its notification. Delivery is at-least-once (PRD §0 #5).

CREATE TABLE webhook_endpoints (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id   uuid NOT NULL REFERENCES merchants(id),
    url           text NOT NULL CHECK (char_length(url) <= 2048),
    secret_enc    bytea NOT NULL,                  -- AES-256-GCM sealed signing secret; never plaintext at rest
    secret_hint   text NOT NULL,                   -- last 4 chars only, for display
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_by    uuid REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    disabled_at   timestamptz
);
CREATE INDEX webhook_endpoints_merchant_idx ON webhook_endpoints (merchant_id, created_at DESC);

-- One event per (type, resource): a payment reaches a given final status once, so a duplicate
-- gateway notification can never produce a second merchant notification.
CREATE TABLE webhook_events (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id   uuid NOT NULL REFERENCES merchants(id),
    type          text NOT NULL,
    resource_id   uuid NOT NULL,
    data          jsonb NOT NULL,                  -- minimum data, no secrets / unneeded PII
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (type, resource_id)
);
CREATE TRIGGER webhook_events_append_only BEFORE UPDATE OR DELETE ON webhook_events
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE webhook_deliveries (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id      uuid NOT NULL REFERENCES merchants(id),
    event_id         uuid NOT NULL REFERENCES webhook_events(id),
    endpoint_id      uuid NOT NULL REFERENCES webhook_endpoints(id),
    redelivery_of    uuid REFERENCES webhook_deliveries(id),
    status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','succeeded','dead','cancelled')),
    attempts         integer NOT NULL DEFAULT 0,
    next_attempt_at  timestamptz NOT NULL DEFAULT now(),   -- also the lease while an attempt is in flight
    last_http_status integer,
    last_latency_ms  integer,
    last_error       text,
    delivered_at     timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'succeeded') = (delivered_at IS NOT NULL))
);
-- Automatic fan-out happens once per (event, endpoint); manual redeliveries are separate rows.
CREATE UNIQUE INDEX webhook_deliveries_fanout_uq ON webhook_deliveries (event_id, endpoint_id) WHERE redelivery_of IS NULL;
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries (next_attempt_at) WHERE status = 'pending';
CREATE INDEX webhook_deliveries_merchant_idx ON webhook_deliveries (merchant_id, created_at DESC);

CREATE TABLE webhook_delivery_attempts (
    id           bigserial PRIMARY KEY,
    delivery_id  uuid NOT NULL REFERENCES webhook_deliveries(id),
    attempt_no   integer NOT NULL,
    http_status  integer,
    latency_ms   integer NOT NULL,
    error        text,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webhook_delivery_attempts_idx ON webhook_delivery_attempts (delivery_id, attempt_no);
CREATE TRIGGER webhook_delivery_attempts_append_only BEFORE UPDATE OR DELETE ON webhook_delivery_attempts
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

INSERT INTO permissions (code) VALUES ('webhook:manage');
INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, 'webhook:manage' FROM roles r WHERE r.code IN ('merchant_owner','integrator');

-- +goose Down
DELETE FROM role_permissions WHERE permission_code = 'webhook:manage';
DELETE FROM permissions WHERE code = 'webhook:manage';
DROP TABLE webhook_delivery_attempts;
DROP TABLE webhook_deliveries;
DROP TABLE webhook_events;
DROP TABLE webhook_endpoints;
