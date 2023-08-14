-- Update the payments table.

-- +migrate Up

ALTER TABLE public.payments
    DROP COLUMN started_at,
    DROP COLUMN completed_at,
    DROP COLUMN old_status,
    DROP COLUMN status_message;


-- +migrate Down

ALTER TABLE public.payments
    ADD COLUMN started_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN completed_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN old_status VARCHAR(16),
    ADD COLUMN status_message VARCHAR(256);