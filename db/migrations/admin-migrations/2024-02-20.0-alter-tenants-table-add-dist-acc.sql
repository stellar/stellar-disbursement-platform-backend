-- +migrate Up
ALTER TABLE
    public.tenants ADD COLUMN distribution_account TEXT NULL;

-- +migrate Down
ALTER TABLE
    public.tenants DROP COLUMN distribution_account;
