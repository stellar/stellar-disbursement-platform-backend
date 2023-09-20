-- +migrate Up

ALTER TABLE public.wallets
    ADD COLUMN enabled boolean NOT NULL DEFAULT true;

-- +migrate Down

ALTER TABLE public.wallets
    DROP COLUMN enabled;
