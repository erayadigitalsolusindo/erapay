-- +goose Up
-- Payment aggregate + append-only event log (PRD payment.md, data.md). Money = bigint minor units.
-- Payment status is separate from settlement/refund status (PRD §0 #9): those get their own tables later.

CREATE TABLE payments (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id        uuid NOT NULL REFERENCES merchants(id),
    outlet_id          uuid REFERENCES merchant_outlets(id),
    merchant_reference text NOT NULL CHECK (char_length(merchant_reference) BETWEEN 1 AND 64),
    description        text CHECK (char_length(description) <= 255),
    amount             bigint NOT NULL CHECK (amount > 0),     -- gross, what the customer pays
    currency           text NOT NULL,
    payment_method     text NOT NULL,
    status             text NOT NULL DEFAULT 'created' CHECK (status IN
        ('created','pending','succeeded','failed','expired','cancelled','unknown')),
    -- Routing choice + gateway-side identifiers (opaque to the domain).
    gateway            text NOT NULL,
    gateway_reference  text,
    gateway_raw_status text,                                   -- raw status kept apart from normalized `status`
    -- Immutable fee snapshot (PRD §0 #8). Server-computed, never client input.
    fee_bearer         text NOT NULL DEFAULT 'merchant' CHECK (fee_bearer = 'merchant'),
    mdr_fee            bigint NOT NULL CHECK (mdr_fee >= 0),
    platform_fee       bigint NOT NULL CHECK (platform_fee >= 0),
    merchant_net       bigint NOT NULL CHECK (merchant_net >= 0),
    pricing_plan_id    uuid NOT NULL REFERENCES pricing_plans(id),
    plan_version       integer NOT NULL,
    expires_at         timestamptz,
    final_at           timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    version            integer NOT NULL DEFAULT 1,
    CHECK (amount = mdr_fee + platform_fee + merchant_net),
    CHECK ((status IN ('succeeded','failed','expired','cancelled')) = (final_at IS NOT NULL))
);
CREATE INDEX payments_merchant_created_idx ON payments (merchant_id, created_at DESC);
CREATE INDEX payments_merchant_ref_idx ON payments (merchant_id, merchant_reference);
CREATE INDEX payments_status_idx ON payments (status) WHERE status IN ('created','pending','unknown');
CREATE UNIQUE INDEX payments_gateway_ref_uq ON payments (gateway, gateway_reference) WHERE gateway_reference IS NOT NULL;

-- Every transition attempt is recorded. applied=false rows are late/conflicting reports
-- (e.g. "succeeded" arriving after "expired") kept for reconciliation, never silently dropped.
CREATE TABLE payment_events (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id         uuid NOT NULL REFERENCES payments(id),
    from_status        text NOT NULL,
    to_status          text NOT NULL,
    applied            boolean NOT NULL,
    source             text NOT NULL CHECK (source IN ('internal','gateway_response','webhook','inquiry')),
    reason             text,
    raw_gateway_status text,
    dedup_key          text UNIQUE,                  -- same webhook/inquiry result processed once
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX payment_events_payment_idx ON payment_events (payment_id, created_at);

CREATE TRIGGER payment_events_append_only BEFORE UPDATE OR DELETE ON payment_events
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
-- Payments are never hard-deleted (PRD §0 #6).
-- +goose StatementBegin
CREATE FUNCTION forbid_delete() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'DELETE on % is not allowed', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER payments_no_delete BEFORE DELETE ON payments
    FOR EACH ROW EXECUTE FUNCTION forbid_delete();

-- The money snapshot and ownership never change after creation (PRD §0 #8).
-- +goose StatementBegin
CREATE FUNCTION payments_immutable_snapshot() RETURNS trigger AS $$
BEGIN
    IF NEW.merchant_id IS DISTINCT FROM OLD.merchant_id OR NEW.amount <> OLD.amount
       OR NEW.currency <> OLD.currency OR NEW.mdr_fee <> OLD.mdr_fee
       OR NEW.platform_fee <> OLD.platform_fee OR NEW.merchant_net <> OLD.merchant_net
       OR NEW.pricing_plan_id <> OLD.pricing_plan_id OR NEW.plan_version <> OLD.plan_version
       OR NEW.gateway <> OLD.gateway OR NEW.fee_bearer <> OLD.fee_bearer THEN
        RAISE EXCEPTION 'payment % snapshot is immutable', OLD.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER payments_immutable_snapshot BEFORE UPDATE ON payments
    FOR EACH ROW EXECUTE FUNCTION payments_immutable_snapshot();

-- +goose Down
DROP TABLE payment_events;
DROP TABLE payments;
DROP FUNCTION payments_immutable_snapshot();
DROP FUNCTION forbid_delete();
