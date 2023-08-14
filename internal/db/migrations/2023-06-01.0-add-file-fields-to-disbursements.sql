-- This is to add `file_name` and `file_content` to `disbursements` table.

-- +migrate Up

ALTER TABLE disbursements
    ADD COLUMN file_content BYTEA NULL,
    ADD COLUMN file_name TEXT NULL;

-- +migrate Down

ALTER TABLE disbursements
    DROP COLUMN file_content,
    DROP COLUMN file_name;

