-- +migrate Up

ALTER TABLE submitter_transactions
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN xdr_sent TEXT UNIQUE,
    ADD COLUMN xdr_received TEXT UNIQUE,
    ALTER COLUMN external_id SET NOT NULL,
    ALTER COLUMN status SET DEFAULT 'PENDING',
    ALTER COLUMN amount TYPE NUMERIC(19,7),
    ADD CONSTRAINT unique_stellar_transaction_hash UNIQUE (stellar_transaction_hash),
    ADD CONSTRAINT check_retry_count CHECK (retry_count >= 0);

CREATE UNIQUE INDEX idx_unique_external_id ON submitter_transactions (external_id) WHERE status != 'ERROR';

-- TRIGGER: updated_at
CREATE TRIGGER refresh_submitter_transactions_updated_at BEFORE UPDATE ON submitter_transactions FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down

-- TRIGGER: updated_at
DROP TRIGGER refresh_submitter_transactions_updated_at ON submitter_transactions;

DROP INDEX idx_unique_external_id;

ALTER TABLE submitter_transactions
    DROP COLUMN updated_at,
    DROP COLUMN xdr_sent,
    DROP COLUMN xdr_received,
    ALTER COLUMN external_id DROP NOT NULL,
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN amount TYPE NUMERIC(10,2);
