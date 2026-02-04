-- +migrate Up

ALTER TABLE embedded_wallets
ADD COLUMN credential_id VARCHAR(64);

CREATE UNIQUE INDEX embedded_wallets_credential_id_key ON embedded_wallets (credential_id) WHERE credential_id IS NOT NULL;

-- +migrate Down

DROP INDEX IF EXISTS embedded_wallets_credential_id_key;

ALTER TABLE embedded_wallets
DROP COLUMN credential_id;