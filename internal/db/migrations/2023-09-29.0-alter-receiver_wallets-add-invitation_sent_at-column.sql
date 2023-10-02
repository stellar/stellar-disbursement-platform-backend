-- +migrate Up

ALTER TABLE public.receiver_wallets
    ADD COLUMN invitation_sent_at timestamp with time zone;

-- +migrate Down

ALTER TABLE public.receiver_wallets
    DROP COLUMN invitation_sent_at;
