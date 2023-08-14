-- This is to update the channel_accounts table with the locked_until_ledger_number column, for concurrent use.
-- It also deletes the unused heartbeat column and add updated_at and locked_at for improved debuggability.

-- +migrate Up
ALTER TABLE public.channel_accounts
    DROP COLUMN heartbeat,
    ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    ADD COLUMN locked_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN locked_until_ledger_number INTEGER;

-- column updated_at
CREATE TRIGGER refresh_channel_accounts_updated_at BEFORE UPDATE ON public.channel_accounts FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

ALTER TABLE public.submitter_transactions
    ADD COLUMN locked_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN locked_until_ledger_number INTEGER,
    DROP COLUMN memo,
    DROP COLUMN memo_type;

-- +migrate Down
DROP TRIGGER refresh_channel_accounts_updated_at ON public.channel_accounts;

ALTER TABLE public.submitter_transactions
    DROP COLUMN locked_at,
    DROP COLUMN locked_until_ledger_number,
    ADD COLUMN memo VARCHAR(64),
    ADD COLUMN memo_type VARCHAR(12);

ALTER TABLE public.channel_accounts
    ADD COLUMN heartbeat TIMESTAMP WITH TIME ZONE,
    DROP COLUMN updated_at,
    DROP COLUMN locked_at,
    DROP COLUMN locked_until_ledger_number;
