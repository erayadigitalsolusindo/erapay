-- +goose Up
-- Optional MDR cap, e.g. the Bank Indonesia QRIS rule: MDR 0.7% capped at Rp500 for
-- Rp0-Rp100.000, effective 1 Nov 2026 (docs/integrations/doku.md §6, docs/prd/pricing.md).
-- 0 = uncapped, so existing plans and today's behaviour are unchanged until a capped
-- plan version is assigned (plan changes are new versions + new assignments, never updates).
ALTER TABLE pricing_plans
    ADD COLUMN mdr_cap       bigint NOT NULL DEFAULT 0 CHECK (mdr_cap >= 0),
    ADD COLUMN mdr_cap_up_to bigint NOT NULL DEFAULT 0 CHECK (mdr_cap_up_to >= 0);

-- +goose Down
ALTER TABLE pricing_plans
    DROP COLUMN mdr_cap_up_to,
    DROP COLUMN mdr_cap;
