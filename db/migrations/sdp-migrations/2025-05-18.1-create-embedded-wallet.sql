-- +migrate Up

CREATE TYPE embedded_wallet_status AS ENUM(
    'PENDING',
    'PROCESSING',
    'FAILED',
    'SUCCESS'
);

CREATE TABLE embedded_wallets (
    token VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    tenant_id VARCHAR(36) NOT NULL,
    wasm_hash VARCHAR(64),
    contract_address VARCHAR(56),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    wallet_status embedded_wallet_status NOT NULL DEFAULT 'PENDING'::embedded_wallet_status
);

-- +migrate Down

DROP TABLE embedded_wallets CASCADE;

DROP TYPE embedded_wallet_status;