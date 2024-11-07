-- add a migration script that adds `user_managed` boolean column to wallets table with default value of false

-- +migrate Up
ALTER TABLE wallets
    ADD COLUMN user_managed BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX idx_unique_user_managed_wallet
    ON Wallets (user_managed)
    WHERE user_managed IS TRUE;

-- +migrate Down
DROP INDEX idx_unique_user_managed_wallet;

ALTER TABLE wallets
    DROP COLUMN user_managed;