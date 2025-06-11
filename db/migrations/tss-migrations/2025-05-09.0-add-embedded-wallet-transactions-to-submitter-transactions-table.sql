-- +migrate Up

CREATE TYPE transaction_type AS ENUM ('PAYMENT', 'WALLET_CREATION', 'SPONSORED');

ALTER TABLE submitter_transactions
    ADD COLUMN transaction_type transaction_type NOT NULL DEFAULT 'PAYMENT'::transaction_type,
    -- Add new columns for wallet creation transactions
    ADD COLUMN public_key VARCHAR(256),
    ADD COLUMN wasm_hash VARCHAR(64),
    -- Add new columns for sponsored transactions
    ADD COLUMN sponsored_account VARCHAR(56),
    ADD COLUMN sponsored_transaction_xdr TEXT;

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
                (asset_issuer IS NOT NULL OR asset_code = 'XLM') AND
                amount IS NOT NULL AND
                destination IS NOT NULL
            WHEN transaction_type = 'WALLET_CREATION' THEN
                public_key IS NOT NULL AND
                wasm_hash IS NOT NULL
            WHEN transaction_type = 'SPONSORED' THEN
                sponsored_account IS NOT NULL AND
                sponsored_transaction_xdr IS NOT NULL
        END
    );

-- +migrate Down

ALTER TABLE submitter_transactions
    DROP CONSTRAINT submitter_transactions_type_constraints;

ALTER TABLE submitter_transactions
    DROP COLUMN public_key,
    DROP COLUMN wasm_hash,
    DROP COLUMN transaction_type,
    DROP COLUMN sponsored_account,
    DROP COLUMN sponsored_transaction_xdr;

ALTER TABLE submitter_transactions
    ALTER COLUMN asset_code SET NOT NULL,
    ALTER COLUMN asset_issuer SET NOT NULL,
    ALTER COLUMN amount SET NOT NULL,
    ALTER COLUMN destination SET NOT NULL;

DROP TYPE transaction_type;