-- This migration adds the memo and memo_type (with custom type) columns to the submitter_transactions table.
-- +migrate Up

-- +migrate StatementBegin
DO $$ BEGIN
    CREATE TYPE memo_type AS ENUM (
        'text',
        'id',
        'hash',
        'return'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;
-- +migrate StatementEnd

ALTER TABLE submitter_transactions
    ADD COLUMN memo TEXT,
    ADD COLUMN memo_type memo_type;

-- +migrate Down


ALTER TABLE submitter_transactions
    DROP COLUMN memo,
    DROP COLUMN memo_type;

DROP TYPE IF EXISTS memo_type;
