-- +goose Up
-- Merchant identity data ("data diri") for verification, DOKU-style (decision 14 Okt 2026).
-- Self sign-up now collects the owner's identity: NIK, place/date of birth, gender, address,
-- plus NPWP and NIB for business accounts. Approval (onboarding.Approve) is refused with
-- IDENTITY_INCOMPLETE until the data is complete.
-- NIK and NPWP are strong identifiers: they are sealed with AES-256-GCM (secretbox, AAD = the
-- merchant row id, same scheme as merchant_bank_accounts.account_enc) and only ever returned
-- masked ("••••1234"); the plaintext is opened only for the reviewer detail view. Place/date of
-- birth, gender, address and NIB stay in cleartext columns because manual review needs to read
-- them (PRD data.md: dokumen KYC mentah dihindari, review masih manual dari data isian).
-- All columns are nullable: merchants created before this migration (and ARUS-provisioned
-- merchants) complete their data via POST /v1/merchant/identity.

ALTER TABLE merchants
    ADD COLUMN owner_id_enc        bytea, -- sealed NIK (16 digits)
    ADD COLUMN owner_id_last4      text,  -- "7890" for the masked view "••••7890"
    ADD COLUMN owner_place_of_birth text,
    ADD COLUMN owner_date_of_birth date,
    ADD COLUMN owner_gender        text CHECK (owner_gender IN ('male','female')),
    ADD COLUMN owner_address       text,
    ADD COLUMN owner_npwp_enc      bytea, -- sealed owner NPWP (15-16 digits), optional
    ADD COLUMN owner_npwp_last4    text,
    ADD COLUMN business_npwp_enc   bytea, -- sealed business NPWP, required for account_type='business'
    ADD COLUMN business_npwp_last4 text,
    ADD COLUMN business_nib        text;  -- NIB (13 digits), required for account_type='business'

-- +goose Down
ALTER TABLE merchants
    DROP COLUMN business_nib,
    DROP COLUMN business_npwp_last4,
    DROP COLUMN business_npwp_enc,
    DROP COLUMN owner_npwp_last4,
    DROP COLUMN owner_npwp_enc,
    DROP COLUMN owner_address,
    DROP COLUMN owner_gender,
    DROP COLUMN owner_date_of_birth,
    DROP COLUMN owner_place_of_birth,
    DROP COLUMN owner_id_last4,
    DROP COLUMN owner_id_enc;
