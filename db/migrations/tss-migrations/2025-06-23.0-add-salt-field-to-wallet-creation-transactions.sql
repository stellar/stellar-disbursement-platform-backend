-- +migrate Up

ALTER TABLE submitter_transactions
    ADD COLUMN salt VARCHAR(64);

ALTER TABLE submitter_transactions
    DROP CONSTRAINT submitter_transactions_wallet_creation_constraints;

ALTER TABLE submitter_transactions
    ADD CONSTRAINT submitter_transactions_wallet_creation_constraints CHECK (
        transaction_type != 'WALLET_CREATION' OR (
            public_key IS NOT NULL AND
            wasm_hash IS NOT NULL AND
            salt IS NOT NULL
        )
    );

-- +migrate Down

ALTER TABLE submitter_transactions
    DROP CONSTRAINT submitter_transactions_wallet_creation_constraints;

ALTER TABLE submitter_transactions
    ADD CONSTRAINT submitter_transactions_wallet_creation_constraints CHECK (
        transaction_type != 'WALLET_CREATION' OR (
            public_key IS NOT NULL AND
            wasm_hash IS NOT NULL
        )
    );

ALTER TABLE submitter_transactions
    DROP COLUMN salt;