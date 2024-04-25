-- +migrate Up
ALTER TABLE
    tenants ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;

-- +migrate Down
ALTER TABLE
    tenants DROP COLUMN deleted_at;
