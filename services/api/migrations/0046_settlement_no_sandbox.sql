-- +goose Up
-- Sandbox payments never settle against real gateway reports (docs/prd/sandbox.md, stage 5). The service
-- refuses them already; this makes the rule hold even for a direct write.
-- +goose StatementBegin
CREATE FUNCTION forbid_sandbox_settlement() RETURNS trigger AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM merchants WHERE id = NEW.merchant_id AND is_sandbox) THEN
        RAISE EXCEPTION 'sandbox merchant % cannot be settled', NEW.merchant_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER settlement_items_no_sandbox BEFORE INSERT ON settlement_items
    FOR EACH ROW EXECUTE FUNCTION forbid_sandbox_settlement();

-- +goose Down
DROP TRIGGER settlement_items_no_sandbox ON settlement_items;
DROP FUNCTION forbid_sandbox_settlement();
