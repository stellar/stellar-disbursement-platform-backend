-- +migrate Up
ALTER TABLE auth_user_mfa_codes
    ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;

-- The primary key is (device_id, auth_user_id), so an auth_user_id-only filter cannot use it.
-- Index auth_user_id to keep per-user operations bounded.
CREATE INDEX idx_auth_user_mfa_codes_auth_user_id ON auth_user_mfa_codes (auth_user_id);

-- +migrate Down
DROP INDEX idx_auth_user_mfa_codes_auth_user_id;

ALTER TABLE auth_user_mfa_codes
    DROP COLUMN attempts;
