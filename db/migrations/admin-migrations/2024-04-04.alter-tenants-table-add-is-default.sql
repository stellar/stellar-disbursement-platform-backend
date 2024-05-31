-- +migrate Up
ALTER TABLE
    tenants ADD COLUMN is_default BOOLEAN DEFAULT false NOT NULL;

-- +migrate Down
ALTER TABLE
    tenants DROP COLUMN is_default;
