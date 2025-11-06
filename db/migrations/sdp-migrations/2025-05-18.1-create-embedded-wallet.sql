-- +migrate Up

CREATE TYPE embedded_wallet_status AS ENUM(
    'PENDING',
    'PROCESSING',
    'FAILED',
    'SUCCESS'
);

CREATE TABLE embedded_wallets (
    token VARCHAR(36) PRIMARY KEY,
    wasm_hash VARCHAR(64),
    contract_address VARCHAR(56),
    public_key VARCHAR(130),
    receiver_wallet_id VARCHAR(36) REFERENCES receiver_wallets (id),
    verification_field verification_type,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    wallet_status embedded_wallet_status NOT NULL DEFAULT 'PENDING'::embedded_wallet_status
);

CREATE TRIGGER refresh_embedded_wallets_updated_at BEFORE UPDATE ON embedded_wallets FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- +migrate Down

DROP TRIGGER refresh_embedded_wallets_updated_at ON embedded_wallets;

DROP TABLE embedded_wallets CASCADE;

DROP TYPE embedded_wallet_status;
