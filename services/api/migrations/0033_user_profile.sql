-- +goose Up
-- Profil pengguna yang bisa diubah sendiri di /dashboard/profil (nama tampilan + telepon).
ALTER TABLE users
    ADD COLUMN full_name text CHECK (full_name IS NULL OR char_length(full_name) BETWEEN 1 AND 100),
    ADD COLUMN phone     text CHECK (phone IS NULL OR char_length(phone) BETWEEN 5 AND 32);

-- +goose Down
ALTER TABLE users DROP COLUMN phone, DROP COLUMN full_name;
