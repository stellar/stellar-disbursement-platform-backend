-- +migrate Up

ALTER TABLE public.submitter_transactions
    ALTER COLUMN amount TYPE NUMERIC(19,7);

-- +migrate Down
