-- +migrate Up

-- WebAuthn credential IDs can be up to 1023 bytes, and after URL encoding they can be longer
ALTER TABLE embedded_wallets
ALTER COLUMN credential_id TYPE VARCHAR(2048);

-- +migrate Down

ALTER TABLE embedded_wallets
ALTER COLUMN credential_id TYPE VARCHAR(64);