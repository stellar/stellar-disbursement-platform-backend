-- This migration adds a new column `otp_attempts` to the `receiver_wallets` table.
-- +migrate Up

ALTER TABLE receiver_wallets
    ADD COLUMN otp_attempts INTEGER NOT NULL DEFAULT 0;

-- +migrate Down
ALTER TABLE receiver_wallets
    DROP COLUMN otp_attempts;
