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

DROP TYPE transaction_type;