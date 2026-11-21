-- +goose Up
-- Owner decision (21 Nov 2026): the platform may permanently delete a tenant together with its financial records.
-- Append-only triggers still block every UPDATE and every other DELETE; a DELETE passes only inside a transaction
-- that sets erapay.purge_merchant = 'on' (auth.Service.DeleteMerchant, which requires the typed tenant name and
-- writes the audit row). audit_logs is never purgeable.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION forbid_mutation() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' AND TG_TABLE_NAME <> 'audit_logs' AND current_setting('erapay.purge_merchant', true) = 'on' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION '% on % is not allowed (append-only)', TG_OP, TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION forbid_delete() RETURNS trigger AS $$
BEGIN
    IF current_setting('erapay.purge_merchant', true) = 'on' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'DELETE on % is not allowed', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION forbid_unless_purge() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' AND (current_setting('erapay.purge_payment', true) = 'on'
                          OR current_setting('erapay.purge_merchant', true) = 'on') THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION '% on % is not allowed (append-only)', TG_OP, TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION forbid_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% on % is not allowed (append-only)', TG_OP, TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION forbid_delete() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'DELETE on % is not allowed', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
CREATE OR REPLACE FUNCTION forbid_unless_purge() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' AND current_setting('erapay.purge_payment', true) = 'on' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION '% on % is not allowed (append-only)', TG_OP, TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
