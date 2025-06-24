-- +migrate Up

CREATE TYPE contact_type AS ENUM (
    'EMAIL',
    'PHONE_NUMBER'
);

ALTER TABLE embedded_wallets
    ADD COLUMN receiver_contact VARCHAR(255) NOT NULL,
    ADD COLUMN contact_type contact_type NOT NULL DEFAULT 'EMAIL';

-- +migrate Down

ALTER TABLE embedded_wallets
    DROP COLUMN receiver_contact,
    DROP COLUMN contact_type;

DROP TYPE IF EXISTS contact_type;