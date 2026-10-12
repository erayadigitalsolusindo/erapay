-- +goose Up
-- Self-service merchant sign-up: a registered merchant stays 'submitted' (cannot take payments or
-- request payouts, both already require 'approved') until a platform admin reviews it.

ALTER TABLE merchants
    ADD COLUMN account_type  text CHECK (account_type IN ('individual','business')),
    ADD COLUMN owner_name    text,
    ADD COLUMN contact_phone text,
    ADD COLUMN review_note   text,
    ADD COLUMN reviewed_by   uuid REFERENCES users(id),
    ADD COLUMN reviewed_at   timestamptz;
CREATE INDEX merchants_status_idx ON merchants (onboarding_status, created_at);

-- Per-IP sign-up throttle (the login throttle in auth_attempts is keyed by email and must not be mixed in).
CREATE TABLE register_attempts (
    id         bigserial PRIMARY KEY,
    ip         text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX register_attempts_ip_idx ON register_attempts (ip, created_at);

-- Dedicated codes: merchant_owner already holds 'merchant:read', so reusing it would expose the
-- cross-tenant review queue to every merchant. Approval sets fees, so it stays with super_admin.
INSERT INTO permissions (code) VALUES ('merchant:review'), ('merchant:approve');
INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN (VALUES
    ('super_admin','merchant:review'), ('finance','merchant:review'), ('ops_support','merchant:review'),
    ('super_admin','merchant:approve')
) AS p(role_code, code) ON p.role_code = r.code;

-- +goose Down
DELETE FROM role_permissions WHERE permission_code IN ('merchant:review','merchant:approve');
DELETE FROM permissions WHERE code IN ('merchant:review','merchant:approve');
DROP TABLE register_attempts;
DROP INDEX merchants_status_idx;
ALTER TABLE merchants
    DROP COLUMN reviewed_at, DROP COLUMN reviewed_by, DROP COLUMN review_note,
    DROP COLUMN contact_phone, DROP COLUMN owner_name, DROP COLUMN account_type;
