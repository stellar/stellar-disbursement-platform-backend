-- +migrate Up
ALTER TABLE receiver_wallets
    ADD COLUMN otp_confirmed_by VARCHAR(256) NULL;

-- +migrate Down
ALTER TABLE receiver_wallets
    DROP COLUMN otp_confirmed_by;