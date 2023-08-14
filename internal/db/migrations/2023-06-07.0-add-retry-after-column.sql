-- This migration adds the `retry_after` column to the `submitter_transactions` table.
-- `retry_after` is used to specify a time in which re-processing of the transaction should not be attempted until after this time.


-- +migrate Up
ALTER TABLE public.submitter_transactions ADD COLUMN retry_after TIMESTAMP;

-- +migrate Down
ALTER TABLE public.submitter_transactions DROP COLUMN retry_after;