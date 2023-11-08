-- This creates the receiver_verifications table that stores the values used to verify a receiver's identity.
-- +migrate Up
CREATE TYPE verification_type AS ENUM (
    'DATE_OF_BIRTH',
    'PIN',
    'NATIONAL_ID_NUMBER');

CREATE TABLE receiver_verifications (
    receiver_id VARCHAR(64) NOT NULL REFERENCES receivers (id) ON DELETE CASCADE,
    verification_field verification_type NOT NULL,
    hashed_value TEXT NOT NULL,
    attempts SMALLINT DEFAULT 0 NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT now() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMP WITH TIME ZONE,
    failed_at TIMESTAMP WITH TIME ZONE,
    PRIMARY KEY (receiver_id, verification_field)
);

-- TRIGGER: updated_at
CREATE TRIGGER refresh_receiver_verifications_updated_at
    BEFORE UPDATE ON receiver_verifications
    FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- Add verification_field to disbursements
ALTER TABLE disbursements
    ADD COLUMN verification_field verification_type NOT NULL DEFAULT 'DATE_OF_BIRTH';

-- Add external_id field
ALTER TABLE receivers
    ADD COLUMN external_id VARCHAR(64);
UPDATE receivers SET external_id = id;
ALTER TABLE receivers ALTER COLUMN external_id SET NOT NULL;

-- Add receiver_wallet_id to payments table
ALTER TABLE payments
    ADD COLUMN receiver_wallet_id VARCHAR(64),
    ADD CONSTRAINT fk_payments_receiver_wallet_id FOREIGN KEY (receiver_wallet_id) REFERENCES receiver_wallets (id);

UPDATE 
    payments p
    SET receiver_wallet_id = (
        SELECT rw.id
        FROM receiver_wallets rw
        WHERE rw.receiver_id = p.receiver_id
        LIMIT 1
    );

ALTER TABLE payments ALTER COLUMN receiver_wallet_id SET NOT NULL;

-- Migrate existing receivers.extra_info into a new receiver_verifications row. This cannot be reverted.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
INSERT
    INTO receiver_verifications (receiver_id, verification_field, confirmed_at, hashed_value)
    (SELECT r.id, 'DATE_OF_BIRTH', r.created_at, public.crypt(TO_CHAR(TO_DATE(r.extra_info, 'MM-YY'), 'YYYY-MM-01'), public.gen_salt('bf', 4)) FROM receivers r);
ALTER TABLE receivers DROP COLUMN extra_info;


-- +migrate Down
ALTER TABLE receivers ADD COLUMN extra_info TEXT;
UPDATE 
    receivers r
    SET extra_info = (
        SELECT rv.hashed_value
        FROM receiver_verifications rv
        WHERE rv.receiver_id = r.id
        AND rv.verification_field = 'DATE_OF_BIRTH'
        LIMIT 1
    );

ALTER TABLE payments DROP COLUMN receiver_wallet_id;

ALTER TABLE receivers DROP COLUMN external_id;

ALTER TABLE disbursements DROP COLUMN verification_field;

DROP TRIGGER refresh_receiver_verifications_updated_at ON receiver_verifications;

DROP TABLE receiver_verifications;

DROP TYPE verification_type;

