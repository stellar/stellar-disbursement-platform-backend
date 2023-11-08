-- +migrate Up

ALTER TABLE auth_users ADD COLUMN roles text[];

-- +migrate Down

ALTER TABLE auth_users DROP COLUMN roles;
