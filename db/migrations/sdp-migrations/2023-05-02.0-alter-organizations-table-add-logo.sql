-- +migrate Up

ALTER TABLE organizations
    ADD COLUMN logo BYTEA NULL;

-- +migrate Down

ALTER TABLE organizations
    DROP COLUMN logo;
