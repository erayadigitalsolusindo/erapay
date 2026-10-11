-- +goose Up
-- Revocable sessions, login attempt tracking (rate limit), and RBAC seed (PRD auth.md).

CREATE TABLE sessions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users(id),
    merchant_id    uuid REFERENCES merchants(id),   -- NULL = platform session; fixed at login
    token_hash     bytea NOT NULL UNIQUE,           -- sha256(opaque token); token itself never stored
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_seen_at   timestamptz NOT NULL DEFAULT now(),
    expires_at     timestamptz NOT NULL,
    revoked_at     timestamptz,
    revoked_reason text,
    ip             text,
    user_agent     text
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE auth_attempts (
    id         bigserial PRIMARY KEY,
    email_key  text NOT NULL,       -- sha256 hex of lower(email); no raw email stored
    ip         text NOT NULL,
    succeeded  boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auth_attempts_email_idx ON auth_attempts (email_key, created_at);
CREATE INDEX auth_attempts_ip_idx ON auth_attempts (ip, created_at);

INSERT INTO permissions (code) VALUES
    ('merchant:read'), ('merchant:write'), ('outlet:read'), ('outlet:write'),
    ('payment:read'), ('payment:create'), ('api_key:manage'), ('user:manage'),
    ('report:read'), ('audit:read'), ('gateway:manage');

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN (VALUES
    ('super_admin','gateway:manage'), ('super_admin','user:manage'), ('super_admin','audit:read'),
    ('super_admin','merchant:read'), ('super_admin','merchant:write'),
    ('ops_support','merchant:read'), ('ops_support','outlet:read'), ('ops_support','payment:read'),
    ('finance','merchant:read'), ('finance','payment:read'), ('finance','report:read'),
    ('merchant_owner','merchant:read'), ('merchant_owner','merchant:write'),
    ('merchant_owner','outlet:read'), ('merchant_owner','outlet:write'),
    ('merchant_owner','payment:read'), ('merchant_owner','payment:create'),
    ('merchant_owner','api_key:manage'), ('merchant_owner','user:manage'), ('merchant_owner','report:read'),
    ('cashier','outlet:read'), ('cashier','payment:read'), ('cashier','payment:create'),
    ('integrator','payment:read'), ('integrator','payment:create')
) AS p(role_code, code) ON p.role_code = r.code;

-- +goose Down
DELETE FROM role_permissions;
DELETE FROM permissions;
DROP TABLE auth_attempts;
DROP TABLE sessions;
