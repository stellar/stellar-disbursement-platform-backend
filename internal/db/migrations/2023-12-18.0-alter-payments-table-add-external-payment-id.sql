-- +migrate Up

ALTER TABLE public.payments
    ADD COLUMN external_payment_id VARCHAR(64) NULL;

-- +migrate Down

ALTER TABLE public.payments
    DROP COLUMN external_payment_id;
