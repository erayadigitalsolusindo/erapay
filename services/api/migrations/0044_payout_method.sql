-- +goose Up
-- The merchant chooses how a payout is paid: 'manual' (Finance transfers by hand within working hours, then mark-paid)
-- or 'auto' (executed through the disbursement gateway). Existing payouts keep 'auto' (both paths worked for them).
ALTER TABLE payouts ADD COLUMN method text NOT NULL DEFAULT 'auto' CHECK (method IN ('manual','auto'));

-- +goose Down
ALTER TABLE payouts DROP COLUMN method;
