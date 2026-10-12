-- +goose Up
-- Settlement (layer 1: gateway -> PT) and reconciliation exceptions (PRD reconciliation.md).
-- Settlement status is its own state, separate from payment status (PRD §0 #9): a payment is
-- `settled` iff it has a settlement_items row. Funds become `available` for payout only then.
-- No gateway report parser exists on purpose: the DOKU report format is unconfirmed [KD];
-- batches are recorded manually by Finance from the official report (PRD: impor manual terkontrol).

CREATE TABLE settlement_batches (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway         text NOT NULL,
    batch_ref       text NOT NULL CHECK (char_length(batch_ref) BETWEEN 1 AND 128),
    currency        text NOT NULL,
    settlement_date date NOT NULL,
    reported_net    bigint NOT NULL CHECK (reported_net > 0),   -- what the official report / bank statement shows
    expected_net    bigint NOT NULL CHECK (expected_net > 0),   -- server-computed: sum(gross - mdr_fee) of the items
    payment_count   integer NOT NULL CHECK (payment_count > 0),
    note            text CHECK (char_length(note) <= 500),
    created_by      uuid REFERENCES users(id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (gateway, batch_ref),                                 -- duplicate-report detection
    CHECK (reported_net = expected_net)                          -- a mismatching batch is never booked
);

CREATE TABLE settlement_items (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id     uuid NOT NULL REFERENCES settlement_batches(id),
    payment_id   uuid NOT NULL UNIQUE REFERENCES payments(id),   -- a payment settles at most once
    merchant_id  uuid NOT NULL REFERENCES merchants(id),
    gross        bigint NOT NULL,
    mdr_fee      bigint NOT NULL,
    expected_net bigint NOT NULL,
    CHECK (expected_net = gross - mdr_fee)
);
CREATE INDEX settlement_items_batch_idx ON settlement_items (batch_id);
CREATE INDEX settlement_items_merchant_idx ON settlement_items (merchant_id);

CREATE TRIGGER settlement_batches_append_only BEFORE UPDATE OR DELETE ON settlement_batches
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER settlement_items_append_only BEFORE UPDATE OR DELETE ON settlement_items
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE reconciliation_exceptions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind            text NOT NULL CHECK (kind IN ('settlement_mismatch')),
    gateway         text NOT NULL,
    reference       text NOT NULL,                    -- e.g. the batch_ref that did not reconcile
    details         jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    resolution_note text CHECK (char_length(resolution_note) <= 1000),
    resolved_by     uuid REFERENCES users(id),
    resolved_at     timestamptz,
    created_by      uuid REFERENCES users(id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'resolved') = (resolved_at IS NOT NULL AND resolution_note IS NOT NULL))
);
-- One open exception per (kind, gateway, reference): repeated attempts do not pile up.
CREATE UNIQUE INDEX reconciliation_exceptions_open_uq ON reconciliation_exceptions (kind, gateway, reference) WHERE status = 'open';

INSERT INTO permissions (code) VALUES ('settlement:read'), ('settlement:manage');
INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN (VALUES
    ('finance','settlement:read'), ('finance','settlement:manage'), ('ops_support','settlement:read')
) AS p(role_code, code) ON p.role_code = r.code;

-- +goose Down
DELETE FROM role_permissions WHERE permission_code IN ('settlement:read','settlement:manage');
DELETE FROM permissions WHERE code IN ('settlement:read','settlement:manage');
DROP TABLE reconciliation_exceptions;
DROP TABLE settlement_items;
DROP TABLE settlement_batches;
