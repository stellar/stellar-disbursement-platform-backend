-- This migration adds the `synced_at` column to the `submitter_transactions` table.
-- `synced_at` is used to track whether a transaction has been synced with the SDP.


-- +migrate Up
ALTER TABLE public.submitter_transactions ADD COLUMN synced_at TIMESTAMP WITH TIME ZONE NULL;

-- +migrate Down
ALTER TABLE public.submitter_transactions DROP COLUMN synced_at;
