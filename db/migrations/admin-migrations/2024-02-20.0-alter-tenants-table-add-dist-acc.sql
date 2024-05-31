-- +migrate Up
ALTER TABLE
    tenants ADD COLUMN distribution_account TEXT NULL;

-- +migrate Down
ALTER TABLE
    tenants DROP COLUMN distribution_account;
