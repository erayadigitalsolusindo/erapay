-- +goose Up
-- Merchant bank accounts and payouts (PRD payout.md, merchant.md). Money = bigint minor units.
-- Funds are RESERVED in the ledger when a payout is requested (merchant_payable -> payout_in_transit),
-- so concurrent requests cannot overspend; they leave the bank only when Finance confirms the transfer.

CREATE TABLE merchant_bank_accounts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id   uuid NOT NULL REFERENCES merchants(id),
    bank_code     text NOT NULL CHECK (char_length(bank_code) BETWEEN 2 AND 20),
    account_enc   bytea NOT NULL,                    -- AES-256-GCM sealed number; bound to the row id
    account_last4 text NOT NULL CHECK (char_length(account_last4) = 4),
    account_name  text NOT NULL CHECK (char_length(account_name) BETWEEN 2 AND 100),
    status        text NOT NULL DEFAULT 'pending_verification' CHECK (status IN ('pending_verification','verified','disabled')),
    created_by    uuid REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    verified_by   uuid REFERENCES users(id),
    verified_at   timestamptz,
    disabled_at   timestamptz,
    CHECK (status <> 'verified' OR (verified_at IS NOT NULL AND verified_by IS NOT NULL)),
    CHECK (verified_by IS NULL OR verified_by IS DISTINCT FROM created_by)    -- creator cannot verify their own account
);
CREATE INDEX merchant_bank_accounts_merchant_idx ON merchant_bank_accounts (merchant_id, created_at DESC);

CREATE TABLE payouts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id     uuid NOT NULL REFERENCES merchants(id),
    bank_account_id uuid NOT NULL REFERENCES merchant_bank_accounts(id),
    amount          bigint NOT NULL CHECK (amount > 0),
    currency        text NOT NULL,
    status          text NOT NULL DEFAULT 'pending_approval' CHECK (status IN ('pending_approval','approved','paid','failed','cancelled')),
    dry_run         boolean NOT NULL,                -- true: no real transfer is expected (non-production)
    requested_by    uuid NOT NULL REFERENCES users(id),
    approved_by     uuid REFERENCES users(id),
    approved_at     timestamptz,
    decision_note   text CHECK (char_length(decision_note) <= 500),
    bank_reference  text CHECK (char_length(bank_reference) <= 128),
    paid_by         uuid REFERENCES users(id),
    paid_at         timestamptz,
    failure_reason  text CHECK (char_length(failure_reason) <= 500),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    version         integer NOT NULL DEFAULT 1,
    CHECK (approved_by IS NULL OR approved_by <> requested_by),                        -- maker != checker
    CHECK (status NOT IN ('approved','paid') OR approved_by IS NOT NULL),
    CHECK ((status = 'paid') = (paid_at IS NOT NULL AND bank_reference IS NOT NULL))
);
CREATE INDEX payouts_merchant_idx ON payouts (merchant_id, created_at DESC);
CREATE INDEX payouts_status_idx ON payouts (status, created_at) WHERE status IN ('pending_approval','approved');

-- Payouts are never deleted; the money fields and the maker never change after creation.
CREATE TRIGGER payouts_no_delete BEFORE DELETE ON payouts FOR EACH ROW EXECUTE FUNCTION forbid_delete();
CREATE TRIGGER bank_accounts_no_delete BEFORE DELETE ON merchant_bank_accounts FOR EACH ROW EXECUTE FUNCTION forbid_delete();
-- +goose StatementBegin
CREATE FUNCTION payouts_immutable_fields() RETURNS trigger AS $$
BEGIN
    IF NEW.merchant_id <> OLD.merchant_id OR NEW.bank_account_id <> OLD.bank_account_id OR NEW.amount <> OLD.amount
       OR NEW.currency <> OLD.currency OR NEW.requested_by <> OLD.requested_by OR NEW.dry_run <> OLD.dry_run THEN
        RAISE EXCEPTION 'payout % money/destination fields are immutable', OLD.id;
    END IF;
    IF OLD.status IN ('paid','failed','cancelled') AND NEW.status <> OLD.status THEN
        RAISE EXCEPTION 'payout % is final', OLD.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER payouts_immutable_fields BEFORE UPDATE ON payouts FOR EACH ROW EXECUTE FUNCTION payouts_immutable_fields();

INSERT INTO permissions (code) VALUES
    ('bank_account:manage'), ('bank_account:verify'), ('payout:request'), ('payout:read'),
    ('payout:review'), ('payout:approve'), ('payout:execute');
INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN (VALUES
    ('merchant_owner','bank_account:manage'), ('merchant_owner','payout:request'), ('merchant_owner','payout:read'),
    ('finance','bank_account:verify'), ('finance','payout:review'), ('finance','payout:approve'), ('finance','payout:execute'),
    ('ops_support','payout:review')
) AS p(role_code, code) ON p.role_code = r.code;

-- +goose Down
DELETE FROM role_permissions WHERE permission_code IN
    ('bank_account:manage','bank_account:verify','payout:request','payout:read','payout:review','payout:approve','payout:execute');
DELETE FROM permissions WHERE code IN
    ('bank_account:manage','bank_account:verify','payout:request','payout:read','payout:review','payout:approve','payout:execute');
DROP TABLE payouts;
DROP FUNCTION payouts_immutable_fields();
DROP TABLE merchant_bank_accounts;
