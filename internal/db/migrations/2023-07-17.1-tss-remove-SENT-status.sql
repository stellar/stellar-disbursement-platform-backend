-- This migration updates the submitter_transactions table by removing the SENT status.

-- +migrate Up
-- Create new type
CREATE TYPE transaction_status_new AS ENUM ('PENDING', 'PROCESSING', 'SUCCESS', 'ERROR');

-- Add a new column with the new type
ALTER TABLE public.submitter_transactions ADD COLUMN status_new transaction_status_new;

-- Copy & transform data
UPDATE public.submitter_transactions SET status_new = 
CASE 
    WHEN status = 'SENT' THEN 'PROCESSING'::transaction_status_new
    ELSE status::text::transaction_status_new
END;

-- Drop the old column
ALTER TABLE public.submitter_transactions DROP COLUMN status;

-- Rename the new column
ALTER TABLE public.submitter_transactions RENAME COLUMN status_new TO status;

-- Drop old type
DROP TYPE transaction_status;

-- Rename new type
ALTER TYPE transaction_status_new RENAME TO transaction_status;

-- Restore index that was when we changed the enum type
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_external_id ON public.submitter_transactions (external_id) WHERE status != 'ERROR';

-- +migrate Down

-- Create old type
CREATE TYPE transaction_status_old AS ENUM ('PENDING', 'PROCESSING', 'SENT', 'SUCCESS', 'ERROR');

-- Add a new column with the old type
ALTER TABLE public.submitter_transactions ADD COLUMN status_old transaction_status_old;

-- Copy data to new column
UPDATE public.submitter_transactions SET status_old = status::text::transaction_status_old;

-- Drop the new column
ALTER TABLE public.submitter_transactions DROP COLUMN status;

-- Rename the old column
ALTER TABLE public.submitter_transactions RENAME COLUMN status_old TO status;

-- Drop new type
DROP TYPE transaction_status;

-- Rename old type
ALTER TYPE transaction_status_old RENAME TO transaction_status;

-- Restore index that was when we changed the enum type
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_external_id ON public.submitter_transactions (external_id) WHERE status != 'ERROR';
