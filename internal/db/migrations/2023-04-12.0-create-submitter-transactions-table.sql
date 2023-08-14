-- This creates the submitter_transactions table
-- +migrate Up

CREATE TYPE transaction_status as enum ('PENDING', 'PROCESSING', 'SENT', 'SUCCESS', 'ERROR');

CREATE TABLE public.submitter_transactions (
    id VARCHAR(64) NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    external_id VARCHAR(64),
    status transaction_status NOT NULL,
    status_message TEXT,
    asset_code  VARCHAR(12) NOT NULL,
    asset_issuer VARCHAR(56) NOT NULL,
    amount numeric(10,2) NOT NULL,
    destination VARCHAR(64) NOT NULL,
    memo VARCHAR(64),
    memo_type VARCHAR(12),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP WITH TIME ZONE,
    sent_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    stellar_transaction_hash VARCHAR(64),
    retry_count INT DEFAULT 0
);

CREATE TABLE public.channel_accounts (
    public_key VARCHAR(64) NOT NULL PRIMARY KEY,
    private_key VARCHAR(64),
    heartbeat TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- +migrate Down

DROP TABLE submitter_transactions;
DROP TABLE channel_accounts;
DROP TYPE transaction_status;