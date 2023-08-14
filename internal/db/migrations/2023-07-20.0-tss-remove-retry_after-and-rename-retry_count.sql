-- This migration removes the unused column retry_after and renames retry_count to attempts_count, updating its value accordingly.
-- Also, updates other columns that were not properly configured.

-- +migrate Up

ALTER TABLE public.submitter_transactions DROP COLUMN retry_after;

ALTER TABLE public.submitter_transactions RENAME COLUMN retry_count TO attempts_count;

UPDATE public.submitter_transactions SET attempts_count = attempts_count + 1 WHERE status != 'PENDING';

--configuring the columns that were not properly configured:
ALTER TABLE public.submitter_transactions
    ALTER COLUMN destination TYPE VARCHAR(56),
    ALTER COLUMN status SET DEFAULT 'PENDING',
    ALTER COLUMN status SET NOT NULL;

-- +migrate Down

ALTER TABLE public.submitter_transactions ADD COLUMN retry_after TIMESTAMPTZ;

ALTER TABLE public.submitter_transactions RENAME COLUMN attempts_count TO retry_count;

UPDATE public.submitter_transactions SET retry_count = retry_count - 1 WHERE status != 'PENDING' AND retry_count > 0;

--reverting configuration for the columns that were not properly configured:
ALTER TABLE public.submitter_transactions
    ALTER COLUMN destination TYPE VARCHAR(64),
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status DROP NOT NULL;
