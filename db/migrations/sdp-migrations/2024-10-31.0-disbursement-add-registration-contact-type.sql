-- This migration adds the receiver_contact_type enum type and the receiver_contact_type column to the disbursements table.

-- +migrate Up
CREATE TYPE registration_contact_types AS ENUM (
    'EMAIL',
    'EMAIL_AND_WALLET_ADDRESS',
    'PHONE_NUMBER',
    'PHONE_NUMBER_AND_WALLET_ADDRESS'
);

ALTER TABLE disbursements
    ADD COLUMN registration_contact_type registration_contact_types;

UPDATE disbursements
    SET registration_contact_type = 'PHONE_NUMBER'
    WHERE registration_contact_type IS NULL;

ALTER TABLE disbursements
    ALTER COLUMN registration_contact_type SET NOT NULL;

-- +migrate Down
ALTER TABLE disbursements
    DROP COLUMN registration_contact_type;

DROP TYPE IF EXISTS registration_contact_types;
