-- +migrate Up

ALTER TABLE receiver_wallets
    ADD COLUMN otp TEXT NULL;
ALTER TABLE receiver_wallets
    ADD COLUMN otp_created_at TIMESTAMP WITH TIME ZONE NULL;
ALTER TABLE receiver_wallets
    ADD COLUMN otp_confirmed_at TIMESTAMP WITH TIME ZONE NULL;

-- +migrate Down

ALTER TABLE receiver_wallets
    DROP COLUMN otp;
ALTER TABLE receiver_wallets
    DROP COLUMN otp_created_at;
ALTER TABLE receiver_wallets
    DROP COLUMN otp_confirmed_at;
