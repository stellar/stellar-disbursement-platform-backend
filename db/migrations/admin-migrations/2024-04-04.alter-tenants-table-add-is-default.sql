-- +migrate Up
ALTER TABLE
    public.tenants ADD COLUMN is_default BOOLEAN DEFAULT false NOT NULL;

-- +migrate Down
ALTER TABLE
    public.tenants DROP COLUMN is_default;
