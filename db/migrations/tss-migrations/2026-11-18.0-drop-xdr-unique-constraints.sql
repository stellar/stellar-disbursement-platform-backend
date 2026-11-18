-- The UNIQUE constraints on xdr_sent and xdr_received limited each envelope to one B-tree index entry (2,704 bytes).
-- stellar_transaction_hash is UNIQUE and already prevents duplicate submissions, and nothing queries these columns.

-- +migrate Up

ALTER TABLE submitter_transactions
    DROP CONSTRAINT submitter_transactions_xdr_sent_key,
    DROP CONSTRAINT submitter_transactions_xdr_received_key;

-- +migrate Down

ALTER TABLE submitter_transactions
    ADD CONSTRAINT submitter_transactions_xdr_sent_key UNIQUE (xdr_sent),
    ADD CONSTRAINT submitter_transactions_xdr_received_key UNIQUE (xdr_received);
