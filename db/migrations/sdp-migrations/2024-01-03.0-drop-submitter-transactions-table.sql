-- +migrate Up

DROP TRIGGER refresh_submitter_transactions_updated_at ON submitter_transactions;

DROP TABLE submitter_transactions;

DROP FUNCTION create_submitter_transactions_status_history;

DROP TYPE transaction_status;


-- +migrate Down

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TYPE transaction_status AS ENUM ('PENDING', 'PROCESSING', 'SUCCESS', 'ERROR');

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION create_submitter_transactions_status_history(time_stamp TIMESTAMP WITH TIME ZONE, tss_status transaction_status, status_message VARCHAR, stellar_transaction_hash TEXT, xdr_sent TEXT, xdr_received TEXT)
RETURNS jsonb AS $$
	BEGIN
	    RETURN json_build_object(
            'timestamp', time_stamp,
            'status', tss_status,
            'status_message', status_message,
            'stellar_transaction_hash', stellar_transaction_hash,
            'xdr_sent', xdr_sent,
            'xdr_received', xdr_received
        );
	END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TABLE submitter_transactions (
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    external_id VARCHAR(64) NOT NULL,

    status transaction_status NOT NULL DEFAULT 'PENDING'::transaction_status,
    status_history jsonb[] NULL DEFAULT ARRAY[create_submitter_transactions_status_history(NOW(), 'PENDING', NULL, NULL, NULL, NULL)],
    status_message TEXT NULL,
    
    asset_code VARCHAR(12) NOT NULL,
    asset_issuer VARCHAR(56) NOT NULL,
    amount NUMERIC(10,7) NOT NULL,
    destination VARCHAR(56) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    synced_at TIMESTAMPTZ,
    locked_until_ledger_number INTEGER,

    stellar_transaction_hash VARCHAR(64) UNIQUE,
    attempts_count integer DEFAULT 0 CHECK (attempts_count >= 0),
    xdr_sent TEXT UNIQUE,
    xdr_received TEXT UNIQUE,

    CONSTRAINT asset_issuer_length_check CHECK ((asset_code = 'XLM' AND char_length(asset_issuer) = 0) OR char_length(asset_issuer) = 56)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_external_id ON submitter_transactions (external_id) WHERE status != 'ERROR';

-- TRIGGER: updated_at
CREATE TRIGGER refresh_submitter_transactions_updated_at BEFORE UPDATE ON submitter_transactions FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();
