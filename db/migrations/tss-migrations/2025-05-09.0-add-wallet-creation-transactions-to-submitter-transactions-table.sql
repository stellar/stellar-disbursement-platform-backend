-- +migrate Up

CREATE TYPE transaction_type AS ENUM ('PAYMENT', 'WALLET_CREATION');

ALTER TABLE submitter_transactions
    ADD COLUMN public_key VARCHAR(256),
    ADD COLUMN wasm_hash VARCHAR(64),
    ADD COLUMN transaction_type VARCHAR(32) NOT NULL DEFAULT 'PAYMENT'::transaction_type;

ALTER TABLE submitter_transactions
    ALTER COLUMN asset_code DROP NOT NULL,
    ALTER COLUMN asset_issuer DROP NOT NULL,
    ALTER COLUMN amount DROP NOT NULL,
    ALTER COLUMN destination DROP NOT NULL;

ALTER TABLE submitter_transactions
    ADD CONSTRAINT submitter_transactions_type_constraints CHECK (
        CASE
            WHEN transaction_type = 'PAYMENT' THEN
                asset_code IS NOT NULL AND
                asset_issuer IS NOT NULL AND
                amount IS NOT NULL AND
                destination IS NOT NULL
            WHEN transaction_type = 'WALLET_CREATION' THEN
                public_key IS NOT NULL AND
                wasm_hash IS NOT NULL
        END
    );

-- +migrate Down

ALTER TABLE submitter_transactions
    DROP COLUMN public_key,
    DROP COLUMN wasm_hash,
    DROP COLUMN transaction_type;

ALTER TABLE submitter_transactions
    ALTER COLUMN asset_code SET NOT NULL,
    ALTER COLUMN asset_issuer SET NOT NULL,
    ALTER COLUMN amount SET NOT NULL,
    ALTER COLUMN destination SET NOT NULL;

ALTER TABLE submitter_transactions
    DROP CONSTRAINT submitter_transactions_type_constraints;

DROP TYPE transaction_type;