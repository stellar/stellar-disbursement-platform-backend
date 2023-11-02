-- +migrate Up

ALTER TABLE channel_accounts
    ALTER COLUMN private_key TYPE VARCHAR(256);

-- +migrate Down

ALTER TABLE channel_accounts
    ALTER COLUMN private_key TYPE VARCHAR(64);
