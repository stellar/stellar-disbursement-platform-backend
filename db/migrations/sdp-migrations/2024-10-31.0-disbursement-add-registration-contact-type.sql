-- This migration adds the receiver_contact_type enum type and the receiver_contact_type column to the disbursements table.

-- +migrate Up
CREATE TYPE registration_contact_types AS ENUM (
    'EMAIL',
    'EMAIL_AND_WALLET_ADDRESS',
    'PHONE_NUMBER',
    'PHONE_NUMBER_AND_WALLET_ADDRESS'
);

-- TODO: create without the NOT NULL constraint, update the existing data, then add the NOT NULL constraint
ALTER TABLE disbursements
    ADD COLUMN registration_contact_type registration_contact_types NOT NULL DEFAULT 'PHONE_NUMBER';

-- +migrate Down
ALTER TABLE disbursements
    DROP COLUMN registration_contact_type;

DROP TYPE IF EXISTS registration_contact_types;
