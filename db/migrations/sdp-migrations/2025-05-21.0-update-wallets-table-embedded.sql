-- +migrate Up

ALTER TABLE wallets
    ADD COLUMN embedded BOOLEAN DEFAULT false;

-- +migrate Down

ALTER TABLE wallets
    DROP COLUMN embedded;
