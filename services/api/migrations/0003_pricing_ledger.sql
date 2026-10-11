-- +goose Up
-- Pricing plans and double-entry ledger (PRD pricing.md, ledger.md). Money = bigint minor units.

-- Plans are immutable: a change = a new version row (no retroactive effect, PRD §0 #8).
CREATE TABLE pricing_plans (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code              text NOT NULL,
    version           integer NOT NULL CHECK (version > 0),
    gateway           text NOT NULL,
    payment_method    text NOT NULL,
    mdr_bps           integer NOT NULL CHECK (mdr_bps BETWEEN 0 AND 10000),
    mdr_fixed         bigint  NOT NULL DEFAULT 0 CHECK (mdr_fixed >= 0),
    mdr_rounding      text NOT NULL CHECK (mdr_rounding IN ('half_up','floor','ceil')),
    platform_bps      integer NOT NULL CHECK (platform_bps BETWEEN 0 AND 10000),
    platform_fixed    bigint  NOT NULL DEFAULT 0 CHECK (platform_fixed >= 0),
    platform_rounding text NOT NULL CHECK (platform_rounding IN ('half_up','floor','ceil')),
    created_by        uuid REFERENCES users(id),
    approved_by       uuid REFERENCES users(id),
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (code, version)
);
CREATE TABLE merchant_pricing_assignments (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id    uuid NOT NULL REFERENCES merchants(id),
    plan_id        uuid NOT NULL REFERENCES pricing_plans(id),
    effective_from timestamptz NOT NULL,
    created_by     uuid REFERENCES users(id),
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, plan_id, effective_from)
);
CREATE INDEX mpa_lookup_idx ON merchant_pricing_assignments (merchant_id, effective_from DESC);

CREATE TABLE ledger_accounts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code        text NOT NULL CHECK (code IN (
        'cash_doku_clearing','cash_bank_pt','merchant_payable','payout_in_transit',
        'refund_payable','platform_fee_revenue','gateway_fee_expense','tax_payable')),
    merchant_id uuid REFERENCES merchants(id),   -- only merchant-scoped accounts
    currency    text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK ((code IN ('merchant_payable','payout_in_transit','refund_payable')) = (merchant_id IS NOT NULL))
);
CREATE UNIQUE INDEX ledger_accounts_uq
    ON ledger_accounts (code, COALESCE(merchant_id, '00000000-0000-0000-0000-000000000000'::uuid), currency);

CREATE TABLE ledger_transactions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    dedup_key   text NOT NULL UNIQUE,       -- same event never posts twice (PRD §0 #5)
    source_type text NOT NULL,              -- e.g. payment_succeeded, payout_created
    source_id   text NOT NULL,
    currency    text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ledger_transactions_source_idx ON ledger_transactions (source_type, source_id);

CREATE TABLE ledger_entries (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    txn_id     uuid NOT NULL REFERENCES ledger_transactions(id),
    account_id uuid NOT NULL REFERENCES ledger_accounts(id),
    direction  text NOT NULL CHECK (direction IN ('debit','credit')),
    amount     bigint NOT NULL CHECK (amount > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ledger_entries_txn_idx ON ledger_entries (txn_id);
CREATE INDEX ledger_entries_account_idx ON ledger_entries (account_id);

-- Append-only (PRD §0 #6). forbid_mutation() is defined in 0001.
CREATE TRIGGER pricing_plans_append_only BEFORE UPDATE OR DELETE ON pricing_plans
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER ledger_transactions_append_only BEFORE UPDATE OR DELETE ON ledger_transactions
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER ledger_entries_append_only BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Every ledger transaction must balance (debits = credits) at commit, and entry
-- accounts must share the transaction currency. Deferred so entries insert one by one.
-- +goose StatementBegin
CREATE FUNCTION check_ledger_txn_balanced() RETURNS trigger AS $$
DECLARE
    d bigint; c bigint; bad int;
BEGIN
    SELECT COALESCE(SUM(amount) FILTER (WHERE direction = 'debit'), 0),
           COALESCE(SUM(amount) FILTER (WHERE direction = 'credit'), 0)
      INTO d, c FROM ledger_entries WHERE txn_id = NEW.txn_id;
    IF d <> c THEN
        RAISE EXCEPTION 'ledger txn % unbalanced: debit % credit %', NEW.txn_id, d, c;
    END IF;
    SELECT count(*) INTO bad
      FROM ledger_entries e
      JOIN ledger_accounts a ON a.id = e.account_id
      JOIN ledger_transactions t ON t.id = e.txn_id
     WHERE e.txn_id = NEW.txn_id AND a.currency <> t.currency;
    IF bad > 0 THEN
        RAISE EXCEPTION 'ledger txn % mixes currencies', NEW.txn_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE CONSTRAINT TRIGGER ledger_entries_balanced
    AFTER INSERT ON ledger_entries DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION check_ledger_txn_balanced();

-- Balances are always derived from entries (PRD §0 #14b).
CREATE VIEW ledger_account_balances AS
SELECT a.id AS account_id, a.code, a.merchant_id, a.currency,
       COALESCE(SUM(CASE e.direction WHEN 'debit' THEN e.amount ELSE -e.amount END), 0) AS debit_minus_credit
  FROM ledger_accounts a LEFT JOIN ledger_entries e ON e.account_id = a.id
 GROUP BY a.id;

-- +goose Down
DROP VIEW ledger_account_balances;
DROP TABLE ledger_entries;
DROP FUNCTION check_ledger_txn_balanced();
DROP TABLE ledger_transactions;
DROP TABLE ledger_accounts;
DROP TABLE merchant_pricing_assignments;
DROP TABLE pricing_plans;
