-- +migrate Up

ALTER TABLE submitter_transactions
    ADD COLUMN credential_id VARCHAR(256),
    ADD COLUMN public_key VARCHAR(128),
    ADD COLUMN wasm_hash VARCHAR(32);

-- Make payment fields nullable
ALTER TABLE submitter_transactions
    ALTER COLUMN external_id DROP NOT NULL,
    ALTER COLUMN asset_code DROP NOT NULL,
    ALTER COLUMN asset_issuer DROP NOT NULL,
    ALTER COLUMN amount DROP NOT NULL,
    ALTER COLUMN destination DROP NOT NULL;

-- +migrate Down

ALTER TABLE submitter_transactions
    DROP COLUMN credential_id,
    DROP COLUMN public_key,
    DROP COLUMN wasm_hash;