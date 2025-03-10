-- This migration updates the type of receiver_wallets.stellar_memo to TEXT, so it fits the length needed for memo_hash.
-- +migrate Up

ALTER TABLE receiver_wallets
    ALTER COLUMN stellar_memo TYPE TEXT;

-- +migrate Down
