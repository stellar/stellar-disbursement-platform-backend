-- This migration drops the verification_field NOT NULL constraint from the disbursements table.

-- +migrate Up
ALTER TABLE disbursements
    ALTER COLUMN verification_field DROP NOT NULL;

-- +migrate Down
