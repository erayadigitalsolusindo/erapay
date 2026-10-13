-- +goose Up
-- MFA (TOTP) for admin users (PRD auth.md: "MFA admin wajib").

-- Add MFA columns to users table
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS mfa_secret text,                     -- base32 encoded TOTP secret (RFC 3548)
    ADD COLUMN IF NOT EXISTS mfa_recovery_codes text[],           -- argon2id hashes of recovery codes (one-time use)
    ADD COLUMN IF NOT EXISTS mfa_enrolled_at timestamptz;         -- when MFA was first enabled

-- Pending enrollment (short-lived, for the enroll -> verify flow)
CREATE TABLE IF NOT EXISTS mfa_pending_enrollment (
    user_id         uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    secret          text NOT NULL,                  -- base32 encoded TOTP secret
    recovery_codes  text[] NOT NULL,                -- plaintext recovery codes (hashed on confirm)
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL
);
CREATE INDEX mfa_pending_expires_idx ON mfa_pending_enrollment (expires_at);

-- MFA challenges (short-lived, for the challenge -> verify flow)
CREATE TABLE IF NOT EXISTS mfa_challenges (
    user_id       uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    challenge_hash text NOT NULL,                   -- sha256 hex of challenge nonce
    expires_at    timestamptz NOT NULL
);
CREATE INDEX mfa_challenges_expires_idx ON mfa_challenges (expires_at);

-- Track MFA attempts for rate limiting
CREATE TABLE IF NOT EXISTS mfa_attempts (
    id            bigserial PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users(id),
    challenge_id  text NOT NULL,                   -- sha256 hex of challenge nonce
    succeeded     boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mfa_attempts_user_idx ON mfa_attempts (user_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS mfa_attempts;
DROP TABLE IF EXISTS mfa_challenges;
DROP TABLE IF EXISTS mfa_pending_enrollment;
ALTER TABLE users
    DROP COLUMN IF EXISTS mfa_secret,
    DROP COLUMN IF EXISTS mfa_recovery_codes,
    DROP COLUMN IF EXISTS mfa_enrolled_at;