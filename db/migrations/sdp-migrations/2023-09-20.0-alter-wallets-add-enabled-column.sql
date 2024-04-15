-- +migrate Up

ALTER TABLE wallets
    ADD COLUMN enabled boolean NOT NULL DEFAULT true;

-- +migrate Down

ALTER TABLE wallets
    DROP COLUMN enabled;
