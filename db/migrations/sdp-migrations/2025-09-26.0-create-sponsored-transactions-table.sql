-- +migrate Up

CREATE TYPE sponsored_transaction_status AS ENUM ('PENDING', 'PROCESSING', 'SUCCESS', 'FAILED');

CREATE TABLE sponsored_transactions (
    id VARCHAR(36) PRIMARY KEY,
    account VARCHAR(56) NOT NULL,
    operation_xdr TEXT NOT NULL,
    status sponsored_transaction_status NOT NULL DEFAULT 'PENDING',
    transaction_hash VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX sponsored_transactions_status_updated_at_idx
    ON sponsored_transactions (status, updated_at);

CREATE TRIGGER refresh_sponsored_transactions_updated_at BEFORE UPDATE ON sponsored_transactions FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- +migrate Down

DROP TRIGGER refresh_sponsored_transactions_updated_at ON sponsored_transactions;

DROP INDEX IF EXISTS sponsored_transactions_status_updated_at_idx;

DROP TABLE sponsored_transactions;
DROP TYPE sponsored_transaction_status;
