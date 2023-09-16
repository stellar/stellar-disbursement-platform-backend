-- This migration updates the submitter_transactions table by addring the status_history table, for increased debuggability.

-- +migrate Up

ALTER TABLE public.receiver_wallets
    ADD COLUMN anchor_platform_transaction_id text;


-- +migrate Down

ALTER TABLE public.receiver_wallets
    DROP COLUMN anchor_platform_transaction_id;
