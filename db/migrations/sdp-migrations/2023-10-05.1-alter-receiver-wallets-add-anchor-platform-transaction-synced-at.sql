-- +migrate Up

ALTER TABLE receiver_wallets
    ADD COLUMN anchor_platform_transaction_synced_at TIMESTAMP WITH TIME ZONE NULL;

-- +migrate Down

ALTER TABLE receiver_wallets
    DROP COLUMN anchor_platform_transaction_synced_at;
