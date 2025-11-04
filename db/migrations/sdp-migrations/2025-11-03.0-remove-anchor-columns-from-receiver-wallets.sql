-- This migration updates the receiver_wallets anchor_platform_transaction_id name to sep24_transaction_id and removes
-- anchor_platform_transaction_synced_at column from both receiver_wallets and its audit table.

-- +migrate Up

-- 1. Drop only the trigger and function
DROP TRIGGER IF EXISTS receiver_wallets_audit_trigger ON receiver_wallets;
DROP FUNCTION IF EXISTS receiver_wallets_audit_fn();

-- 2. Modify the source table schema
ALTER TABLE receiver_wallets RENAME COLUMN anchor_platform_transaction_id TO sep24_transaction_id;
ALTER TABLE receiver_wallets DROP COLUMN anchor_platform_transaction_synced_at;

-- 3. Modify the audit table to match
ALTER TABLE receiver_wallets_audit RENAME COLUMN anchor_platform_transaction_id TO sep24_transaction_id;
ALTER TABLE receiver_wallets_audit DROP COLUMN IF EXISTS anchor_platform_transaction_synced_at;

-- 4. Recreate trigger and function with new schema (audit table already exists, won't be recreated)
SELECT 1 FROM create_audit_table('receiver_wallets');

-- +migrate Down

-- 1. Drop only the trigger and function
DROP TRIGGER IF EXISTS receiver_wallets_audit_trigger ON receiver_wallets;
DROP FUNCTION IF EXISTS receiver_wallets_audit_fn();

-- 2. Restore the source table schema
ALTER TABLE receiver_wallets RENAME COLUMN sep24_transaction_id TO anchor_platform_transaction_id;
ALTER TABLE receiver_wallets ADD COLUMN anchor_platform_transaction_synced_at TIMESTAMP WITH TIME ZONE;

-- 3. Restore the audit table schema
ALTER TABLE receiver_wallets_audit RENAME COLUMN sep24_transaction_id TO anchor_platform_transaction_id;
ALTER TABLE receiver_wallets_audit ADD COLUMN anchor_platform_transaction_synced_at TIMESTAMP WITH TIME ZONE;

-- 4. Recreate trigger and function with original schema
SELECT 1 FROM create_audit_table('receiver_wallets');