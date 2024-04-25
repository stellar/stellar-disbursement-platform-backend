-- +migrate Up
ALTER TABLE
    public.tenants ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;

-- +migrate Down
ALTER TABLE
    public.tenants DROP COLUMN deleted_at;
