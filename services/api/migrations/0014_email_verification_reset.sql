-- +goose Up
-- Email ownership proof and password reset (one-time tokens), plus a shared per-IP / per-email throttle.

ALTER TABLE users ADD COLUMN email_verified_at timestamptz;
-- Every account that exists now was created by an operator (bootstrap), not by self sign-up.
UPDATE users SET email_verified_at = created_at;

-- Only sha256(token) is stored; the token itself exists only in the email.
CREATE TABLE email_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id),
    purpose    text NOT NULL CHECK (purpose IN ('verify_email','reset_password')),
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_tokens_open_idx ON email_tokens (user_id, purpose) WHERE used_at IS NULL;

-- register_attempts becomes the generic throttle table (kind: register, resend, forgot, dup_notice).
ALTER TABLE register_attempts
    ADD COLUMN kind      text NOT NULL DEFAULT 'register',
    ADD COLUMN email_key text;   -- sha256 hex of lower(email); no raw email stored
CREATE INDEX register_attempts_email_idx ON register_attempts (kind, email_key, created_at);

-- +goose Down
DROP INDEX register_attempts_email_idx;
ALTER TABLE register_attempts DROP COLUMN email_key, DROP COLUMN kind;
DROP TABLE email_tokens;
ALTER TABLE users DROP COLUMN email_verified_at;
