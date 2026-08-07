-- +migrate Up
ALTER TABLE auth_user_mfa_codes
    ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;

-- +migrate Down
ALTER TABLE auth_user_mfa_codes
    DROP COLUMN attempts;
