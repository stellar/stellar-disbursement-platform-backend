

-- +migrate Up

ALTER TABLE submitter_transactions
    ADD COLUMN tenant_id VARCHAR(36) NOT NULL,
    ADD COLUMN distribution_account VARCHAR(56);

COMMENT ON COLUMN submitter_transactions.distribution_account IS 
    'This column will be populated when the TSS submits the transaction to the Stellar network, at the same time as the with the stellar_transaction_hash column.';

-- +migrate Down

ALTER TABLE submitter_transactions
    DROP COLUMN tenant_id,
    DROP COLUMN distribution_account;
