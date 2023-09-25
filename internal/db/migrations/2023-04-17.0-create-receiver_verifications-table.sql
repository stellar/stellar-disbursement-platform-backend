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

-- Add external_id field
ALTER TABLE public.receivers
    ADD COLUMN external_id VARCHAR(64);
UPDATE public.receivers SET external_id = id;
ALTER TABLE public.receivers ALTER COLUMN external_id SET NOT NULL;

-- Add receiver_wallet_id to payments table
ALTER TABLE public.payments
    ADD COLUMN receiver_wallet_id VARCHAR(64),
    ADD CONSTRAINT fk_payments_receiver_wallet_id FOREIGN KEY (receiver_wallet_id) REFERENCES public.receiver_wallets (id);

UPDATE 
    public.payments p
    SET receiver_wallet_id = (
        SELECT rw.id
        FROM public.receiver_wallets rw
        WHERE rw.receiver_id = p.receiver_id
        LIMIT 1
    );

ALTER TABLE public.payments ALTER COLUMN receiver_wallet_id SET NOT NULL;

-- Migrate existing receivers.extra_info into a new receiver_verifications row. This cannot be reverted.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
INSERT
    INTO public.receiver_verifications (receiver_id, verification_field, confirmed_at, hashed_value)
    (SELECT r.id, 'DATE_OF_BIRTH', r.created_at, crypt(TO_CHAR(TO_DATE(r.extra_info, 'MM-YY'), 'YYYY-MM-01'), gen_salt('bf', 4)) FROM public.receivers r);
ALTER TABLE public.receivers DROP COLUMN extra_info;


-- +migrate Down
ALTER TABLE public.receivers ADD COLUMN extra_info TEXT;
UPDATE 
    public.receivers r
    SET extra_info = (
        SELECT rv.hashed_value
        FROM public.receiver_verifications rv
        WHERE rv.receiver_id = r.id
        AND rv.verification_field = 'DATE_OF_BIRTH'
        LIMIT 1
    );
DROP EXTENSION IF EXISTS pgcrypto;

ALTER TABLE public.payments DROP COLUMN receiver_wallet_id;

ALTER TABLE public.receivers DROP COLUMN external_id;

ALTER TABLE public.disbursements DROP COLUMN verification_field;

DROP TRIGGER refresh_receiver_verifications_updated_at ON public.receiver_verifications;

DROP TABLE public.receiver_verifications;

DROP TYPE verification_type;

