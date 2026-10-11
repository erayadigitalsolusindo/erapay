-- +goose Up
-- Identity, RBAC, merchants (PRD auth.md, merchant.md). Tenant key = merchants.id (PRD §0 #7).

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL,
    password_hash text NOT NULL,           -- argon2id/bcrypt hash only
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    mfa_enabled   boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    version       integer NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX users_email_lower_uq ON users (lower(email));

CREATE TABLE roles (
    id    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code  text NOT NULL UNIQUE,
    scope text NOT NULL CHECK (scope IN ('platform','merchant'))
);
CREATE TABLE permissions (
    code text PRIMARY KEY
);
CREATE TABLE role_permissions (
    role_id         uuid NOT NULL REFERENCES roles(id),
    permission_code text NOT NULL REFERENCES permissions(code),
    PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE merchants (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    legal_name        text NOT NULL,
    display_name      text NOT NULL,
    category          text,
    onboarding_status text NOT NULL DEFAULT 'draft' CHECK (onboarding_status IN
        ('draft','submitted','under_review','action_required','approved','rejected','suspended')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    version           integer NOT NULL DEFAULT 1
);

-- Per-gateway activation is separate from EraPay onboarding status (merchant.md).
CREATE TABLE merchant_gateway_accounts (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id        uuid NOT NULL REFERENCES merchants(id),
    gateway            text NOT NULL,
    environment        text NOT NULL CHECK (environment IN ('sandbox','production')),
    gateway_ref        text,                 -- e.g. DOKU merchantId, once confirmed
    activation_status  text NOT NULL DEFAULT 'pending_confirmation'
        CHECK (activation_status IN ('pending_confirmation','active','suspended')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, gateway, environment)
);

CREATE TABLE merchant_outlets (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    name        text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX merchant_outlets_merchant_idx ON merchant_outlets (merchant_id);

-- Platform roles: user_roles. Merchant roles: merchant_users (scoped to one merchant).
CREATE TABLE user_roles (
    user_id uuid NOT NULL REFERENCES users(id),
    role_id uuid NOT NULL REFERENCES roles(id),
    PRIMARY KEY (user_id, role_id)
);
CREATE TABLE merchant_users (
    merchant_id uuid NOT NULL REFERENCES merchants(id),
    user_id     uuid NOT NULL REFERENCES users(id),
    role_id     uuid NOT NULL REFERENCES roles(id),
    outlet_id   uuid REFERENCES merchant_outlets(id),
    PRIMARY KEY (merchant_id, user_id)
);
CREATE INDEX merchant_users_user_idx ON merchant_users (user_id);

INSERT INTO roles (code, scope) VALUES
    ('super_admin','platform'), ('ops_support','platform'), ('finance','platform'),
    ('merchant_owner','merchant'), ('cashier','merchant'), ('integrator','merchant');

-- +goose Down
DROP TABLE merchant_users;
DROP TABLE user_roles;
DROP TABLE merchant_outlets;
DROP TABLE merchant_gateway_accounts;
DROP TABLE merchants;
DROP TABLE role_permissions;
DROP TABLE permissions;
DROP TABLE roles;
DROP TABLE users;
