-- +goose Up
-- Owner decision (20 Nov 2026): the platform (DEWA) may hard-delete a payment that never moved money.
-- Triggers still block every other DELETE and every UPDATE: a delete is allowed only inside a transaction that
-- sets erapay.purge_payment = 'on' (payment.Service.Purge, which checks eligibility and writes the audit row).
-- +goose StatementBegin
CREATE FUNCTION forbid_unless_purge() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' AND current_setting('erapay.purge_payment', true) = 'on' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION '% on % is not allowed (append-only)', TG_OP, TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER payments_no_delete ON payments;
CREATE TRIGGER payments_no_delete BEFORE DELETE ON payments
    FOR EACH ROW EXECUTE FUNCTION forbid_unless_purge();
DROP TRIGGER payment_events_append_only ON payment_events;
CREATE TRIGGER payment_events_append_only BEFORE UPDATE OR DELETE ON payment_events
    FOR EACH ROW EXECUTE FUNCTION forbid_unless_purge();
DROP TRIGGER payment_attempts_append_only ON payment_attempts;
CREATE TRIGGER payment_attempts_append_only BEFORE UPDATE OR DELETE ON payment_attempts
    FOR EACH ROW EXECUTE FUNCTION forbid_unless_purge();

-- +goose Down
DROP TRIGGER payments_no_delete ON payments;
CREATE TRIGGER payments_no_delete BEFORE DELETE ON payments
    FOR EACH ROW EXECUTE FUNCTION forbid_delete();
DROP TRIGGER payment_events_append_only ON payment_events;
CREATE TRIGGER payment_events_append_only BEFORE UPDATE OR DELETE ON payment_events
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
DROP TRIGGER payment_attempts_append_only ON payment_attempts;
CREATE TRIGGER payment_attempts_append_only BEFORE UPDATE OR DELETE ON payment_attempts
    FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
DROP FUNCTION forbid_unless_purge();
