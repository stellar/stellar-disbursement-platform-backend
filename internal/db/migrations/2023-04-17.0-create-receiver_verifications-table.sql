-- This creates the receiver_verifications table that stores the values used to verify a receiver's identity.

-- +migrate Up
CREATE TYPE verification_type AS ENUM (
    'DATE_OF_BIRTH',
    'PIN',
    'NATIONAL_ID_NUMBER');

CREATE TABLE public.receiver_verifications (
    receiver_id VARCHAR(64) NOT NULL REFERENCES public.receivers (id) ON DELETE CASCADE,
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
    BEFORE UPDATE ON public.receiver_verifications
    FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- Add verification_field to disbursements
ALTER TABLE public.disbursements
    ADD COLUMN verification_field verification_type NOT NULL DEFAULT 'DATE_OF_BIRTH';

-- Remove PII field from receivers table and add external_id field
ALTER TABLE public.receivers
    DROP COLUMN encrypted_pii,
    ADD COLUMN external_id VARCHAR(64);

-- Add receiver_wallet_id to payments table
ALTER TABLE public.payments
    ADD COLUMN receiver_wallet_id VARCHAR(64) NOT NULL,
    ADD CONSTRAINT fk_payments_receiver_wallet_id FOREIGN KEY (receiver_wallet_id) REFERENCES public.receiver_wallets (id);

-- +migrate Down
DROP TRIGGER refresh_receiver_verifications_updated_at ON public.receiver_verifications;

DROP TABLE public.receiver_verifications;

ALTER TABLE public.disbursements DROP COLUMN verification_field;

DROP TYPE verification_type;

ALTER TABLE public.receivers
    ADD COLUMN encrypted_pii jsonb,
    DROP COLUMN external_id;

ALTER TABLE public.payments
    DROP COLUMN receiver_wallet_id;

