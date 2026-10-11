-- +goose Up
-- Gateway flow support: QR payload, inquiry scheduling, sanitized attempt log (PRD payment.md).

ALTER TABLE payments
    ADD COLUMN qr_payload       text,                       -- provider_defined, only meaningful with its payment
    ADD COLUMN next_inquiry_at  timestamptz,                -- when the poller should ask the gateway next
    ADD COLUMN inquiry_attempts integer NOT NULL DEFAULT 0;
CREATE INDEX payments_inquiry_idx ON payments (next_inquiry_at)
    WHERE status IN ('pending','unknown') AND gateway_reference IS NOT NULL;

-- One row per call to a gateway. Sanitized by construction: no bodies, headers, or secrets.
CREATE TABLE payment_attempts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id   uuid NOT NULL REFERENCES payments(id),
    gateway      text NOT NULL,
    operation    text NOT NULL CHECK (operation IN ('create','inquiry','cancel')),
    outcome      text NOT NULL CHECK (outcome IN ('ok','rejected','ambiguous','unmapped','amount_mismatch')),
    http_status  integer,
    gateway_code text,                                      -- gateway response code, e.g. SNAP responseCode
    raw_status   text,
    latency_ms   integer,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX payment_attempts_payment_idx ON payment_attempts (payment_id, created_at);
CREATE TRIGGER payment_attempts_append_only BEFORE UPDATE OR DELETE ON payment_attempts
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- +goose Down
DROP TABLE payment_attempts;
DROP INDEX payments_inquiry_idx;
ALTER TABLE payments DROP COLUMN qr_payload, DROP COLUMN next_inquiry_at, DROP COLUMN inquiry_attempts;
