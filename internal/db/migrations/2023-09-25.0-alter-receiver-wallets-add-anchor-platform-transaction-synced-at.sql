-- +migrate Up

ALTER TABLE public.receiver_wallets
    ADD COLUMN anchor_platform_transaction_synced_at TIMESTAMP WITH TIME ZONE NULL;

-- +migrate Down

ALTER TABLE public.receiver_wallets
    DROP COLUMN anchor_platform_transaction_synced_at;
