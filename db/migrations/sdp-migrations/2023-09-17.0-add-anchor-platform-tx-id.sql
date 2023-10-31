-- This migration updates the receiver_wallets table by adding the anchor_platform_transaction_id column, for increased debuggability.

-- +migrate Up

ALTER TABLE receiver_wallets
    ADD COLUMN anchor_platform_transaction_id text;


-- +migrate Down

ALTER TABLE receiver_wallets
    DROP COLUMN anchor_platform_transaction_id;
