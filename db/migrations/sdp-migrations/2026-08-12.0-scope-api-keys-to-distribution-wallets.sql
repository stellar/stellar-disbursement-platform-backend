-- Gives each API key its own distribution-wallet scope instead of deriving it from the creator's
-- live wallet_memberships. Empty means no wallet-scoped access (fail closed), never "all".

-- +migrate Up

ALTER TABLE api_keys
    ADD COLUMN distribution_wallet_ids VARCHAR(36) [] NOT NULL DEFAULT '{}';

COMMENT ON COLUMN api_keys.distribution_wallet_ids IS
    'Distribution wallets this key may act on. Empty = no wallet-scoped access, never "all".';

-- Existing keys have only ever transacted on the default wallet, so scoping them to it is the
-- no-change answer. Guarded on the default row existing so this stays a no-op on an unseeded schema.
UPDATE api_keys
SET distribution_wallet_ids = ARRAY [(SELECT id FROM distribution_wallets WHERE is_default LIMIT 1)]
WHERE EXISTS (SELECT 1 FROM distribution_wallets WHERE is_default);

-- The audit trigger writes an explicit column list, so the new column has to reach api_keys_audit
-- too or scope changes go unrecorded. create_audit_table only CREATE TABLE IF NOT EXISTS, so the
-- column is added by hand; re-running it then regenerates the trigger function over all columns.
ALTER TABLE api_keys_audit
    ADD COLUMN distribution_wallet_ids VARCHAR(36) [];

SELECT create_audit_table('api_keys');

-- +migrate Down

ALTER TABLE api_keys
    DROP COLUMN IF EXISTS distribution_wallet_ids;

ALTER TABLE api_keys_audit
    DROP COLUMN IF EXISTS distribution_wallet_ids;

SELECT create_audit_table('api_keys');
