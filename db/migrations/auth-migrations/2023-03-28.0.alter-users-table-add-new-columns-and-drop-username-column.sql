-- +migrate Up

ALTER TABLE auth_users
    ADD COLUMN first_name VARCHAR(128) NOT NULL DEFAULT '',
    ADD COLUMN last_name VARCHAR(128) NOT NULL DEFAULT '';

ALTER TABLE auth_users DROP COLUMN username;

-- +migrate Down

ALTER TABLE auth_users
    DROP COLUMN first_name,
    DROP COLUMN last_name;

ALTER TABLE auth_users
    ADD COLUMN username VARCHAR(128) UNIQUE;
