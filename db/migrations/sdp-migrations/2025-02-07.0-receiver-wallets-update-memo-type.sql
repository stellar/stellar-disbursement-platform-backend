-- This migration updates the stellar_memo_type column to use the memo_type enum (new type).
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

ALTER TABLE receiver_wallets
    ALTER COLUMN stellar_memo_type TYPE memo_type
    USING CASE
        WHEN stellar_memo_type IN ('text', 'id', 'hash', 'return') THEN stellar_memo_type::memo_type
        ELSE 'text'::memo_type
    END;

ALTER TABLE circle_recipients
    ADD COLUMN stellar_address TEXT,
    ADD COLUMN stellar_memo TEXT;


-- +migrate Down

ALTER TABLE circle_recipients
    DROP COLUMN stellar_address,
    DROP COLUMN stellar_memo;

ALTER TABLE receiver_wallets
    ALTER COLUMN stellar_memo_type TYPE TEXT USING stellar_memo_type::text;

DROP TYPE IF EXISTS memo_type;
