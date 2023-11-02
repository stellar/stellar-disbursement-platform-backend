-- +migrate Up

ALTER TABLE auth_users ADD COLUMN is_active boolean DEFAULT true;

-- +migrate Down

ALTER TABLE auth_users DROP COLUMN is_active;
