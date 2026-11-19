-- +goose Up
-- Sandbox per tenant (docs/prd/sandbox.md): a production merchant may own exactly one sandbox "twin".
ALTER TABLE merchants ADD COLUMN is_sandbox boolean NOT NULL DEFAULT false;
ALTER TABLE merchants ADD COLUMN parent_id uuid REFERENCES merchants(id);
ALTER TABLE merchants ADD CONSTRAINT merchants_sandbox_parent CHECK (is_sandbox = (parent_id IS NOT NULL));
CREATE UNIQUE INDEX merchants_parent_uq ON merchants (parent_id) WHERE parent_id IS NOT NULL;

-- Ledger accounts live in one world only: livemode=false accounts never share a journal or a balance
-- with real money. Existing accounts are real (DEFAULT true).
ALTER TABLE ledger_accounts ADD COLUMN livemode boolean NOT NULL DEFAULT true;
DROP INDEX ledger_accounts_uq;
CREATE UNIQUE INDEX ledger_accounts_uq
    ON ledger_accounts (code, COALESCE(merchant_id, '00000000-0000-0000-0000-000000000000'::uuid), currency, livemode);

-- A merchant-scoped account must live in its merchant's world.
-- +goose StatementBegin
CREATE FUNCTION check_ledger_account_livemode() RETURNS trigger AS $$
DECLARE sbx boolean;
BEGIN
    IF NEW.merchant_id IS NOT NULL THEN
        SELECT is_sandbox INTO sbx FROM merchants WHERE id = NEW.merchant_id;
        IF sbx IS DISTINCT FROM (NOT NEW.livemode) THEN
            RAISE EXCEPTION 'ledger account for merchant % has wrong livemode', NEW.merchant_id;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER ledger_accounts_livemode BEFORE INSERT ON ledger_accounts
    FOR EACH ROW EXECUTE FUNCTION check_ledger_account_livemode();

-- One journal, one world: all entries of a transaction must use accounts of the same livemode (deferred, like the balance check).
-- +goose StatementBegin
CREATE FUNCTION check_ledger_txn_livemode() RETURNS trigger AS $$
DECLARE worlds int;
BEGIN
    SELECT count(DISTINCT a.livemode) INTO worlds
      FROM ledger_entries e JOIN ledger_accounts a ON a.id = e.account_id WHERE e.txn_id = NEW.txn_id;
    IF worlds > 1 THEN
        RAISE EXCEPTION 'ledger txn % mixes sandbox and live accounts', NEW.txn_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER ledger_entries_livemode
    AFTER INSERT ON ledger_entries DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_ledger_txn_livemode();

-- Appending the column keeps existing readers working; platform-wide readers must filter livemode.
CREATE OR REPLACE VIEW ledger_account_balances AS
SELECT a.id AS account_id, a.code, a.merchant_id, a.currency,
       COALESCE(SUM(CASE e.direction WHEN 'debit' THEN e.amount ELSE -e.amount END), 0) AS debit_minus_credit,
       a.livemode
  FROM ledger_accounts a LEFT JOIN ledger_entries e ON e.account_id = a.id
 GROUP BY a.id;

-- +goose Down
DROP VIEW ledger_account_balances;
CREATE VIEW ledger_account_balances AS
SELECT a.id AS account_id, a.code, a.merchant_id, a.currency,
       COALESCE(SUM(CASE e.direction WHEN 'debit' THEN e.amount ELSE -e.amount END), 0) AS debit_minus_credit
  FROM ledger_accounts a LEFT JOIN ledger_entries e ON e.account_id = a.id
 GROUP BY a.id;
DROP TRIGGER ledger_entries_livemode ON ledger_entries;
DROP FUNCTION check_ledger_txn_livemode();
DROP TRIGGER ledger_accounts_livemode ON ledger_accounts;
DROP FUNCTION check_ledger_account_livemode();
DROP INDEX ledger_accounts_uq;
ALTER TABLE ledger_accounts DROP COLUMN livemode;
CREATE UNIQUE INDEX ledger_accounts_uq
    ON ledger_accounts (code, COALESCE(merchant_id, '00000000-0000-0000-0000-000000000000'::uuid), currency);
DROP INDEX merchants_parent_uq;
ALTER TABLE merchants DROP CONSTRAINT merchants_sandbox_parent;
ALTER TABLE merchants DROP COLUMN parent_id;
ALTER TABLE merchants DROP COLUMN is_sandbox;
