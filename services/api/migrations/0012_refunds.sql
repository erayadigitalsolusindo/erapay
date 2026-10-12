-- +goose Up
-- Refunds (PRD refund.md): a separate entity and state machine; a succeeded payment never turns into
-- "failed" (PRD §0 #9). No gateway refund adapter exists (DOKU refund is [KD]), so Finance returns the money
-- to the customer out of band and records it. The merchant's funds are RESERVED in the ledger on request
-- (merchant_payable -> refund_payable) and the balance may not go negative.

CREATE TABLE refunds (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id    uuid NOT NULL REFERENCES merchants(id),
    payment_id     uuid NOT NULL REFERENCES payments(id),
    amount         bigint NOT NULL CHECK (amount > 0),
    currency       text NOT NULL,
    reason         text NOT NULL CHECK (char_length(reason) BETWEEN 3 AND 255),
    status         text NOT NULL DEFAULT 'pending_approval' CHECK (status IN ('pending_approval','approved','refunded','failed','cancelled')),
    requested_by   uuid NOT NULL REFERENCES users(id),
    approved_by    uuid REFERENCES users(id),
    approved_at    timestamptz,
    decision_note  text CHECK (char_length(decision_note) <= 500),
    bank_reference text CHECK (char_length(bank_reference) <= 128),  -- Finance's proof the customer was paid back
    executed_by    uuid REFERENCES users(id),
    refunded_at    timestamptz,
    failure_reason text CHECK (char_length(failure_reason) <= 500),
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    version        integer NOT NULL DEFAULT 1,
    CHECK (approved_by IS NULL OR approved_by <> requested_by),
    CHECK (status NOT IN ('approved','refunded') OR approved_by IS NOT NULL),
    CHECK ((status = 'refunded') = (refunded_at IS NOT NULL AND bank_reference IS NOT NULL))
);
CREATE INDEX refunds_merchant_idx ON refunds (merchant_id, created_at DESC);
CREATE INDEX refunds_payment_idx ON refunds (payment_id);
CREATE INDEX refunds_status_idx ON refunds (status, created_at) WHERE status IN ('pending_approval','approved');

CREATE TRIGGER refunds_no_delete BEFORE DELETE ON refunds FOR EACH ROW EXECUTE FUNCTION forbid_delete();
-- +goose StatementBegin
CREATE FUNCTION refunds_immutable_fields() RETURNS trigger AS $$
BEGIN
    IF NEW.merchant_id <> OLD.merchant_id OR NEW.payment_id <> OLD.payment_id OR NEW.amount <> OLD.amount
       OR NEW.currency <> OLD.currency OR NEW.requested_by <> OLD.requested_by OR NEW.reason <> OLD.reason THEN
        RAISE EXCEPTION 'refund % core fields are immutable', OLD.id;
    END IF;
    IF OLD.status IN ('refunded','failed','cancelled') AND NEW.status <> OLD.status THEN
        RAISE EXCEPTION 'refund % is final', OLD.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER refunds_immutable_fields BEFORE UPDATE ON refunds FOR EACH ROW EXECUTE FUNCTION refunds_immutable_fields();

INSERT INTO permissions (code) VALUES
    ('refund:request'), ('refund:read'), ('refund:review'), ('refund:approve'), ('refund:execute');
INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN (VALUES
    ('merchant_owner','refund:request'), ('merchant_owner','refund:read'),
    ('finance','refund:review'), ('finance','refund:approve'), ('finance','refund:execute'),
    ('ops_support','refund:review')
) AS p(role_code, code) ON p.role_code = r.code;

-- +goose Down
DELETE FROM role_permissions WHERE permission_code IN ('refund:request','refund:read','refund:review','refund:approve','refund:execute');
DELETE FROM permissions WHERE code IN ('refund:request','refund:read','refund:review','refund:approve','refund:execute');
DROP TABLE refunds;
DROP FUNCTION refunds_immutable_fields();
