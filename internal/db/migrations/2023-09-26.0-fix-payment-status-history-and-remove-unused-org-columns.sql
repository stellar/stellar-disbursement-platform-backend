-- This migration renames the occurrences of FAILURE into FAILED from the payment status_history column, that were
-- missed in 2023-05-31.0-replace-payment-status-enum.sql +migrate Up. It also removes two unused columns from the
-- organizations table.


-- +migrate Up

-- Remove unused column
ALTER TABLE public.organizations
    DROP COLUMN stellar_main_address,
    DROP COLUMN are_payments_enabled;

-- Update the status_history column to replace `FAILURE` with `FAILED`
WITH to_be_updated_cte AS (
	SELECT
		DISTINCT payments.id
	FROM
		payments, unnest(payments.status_history) AS status_element
	WHERE
		status_element::text LIKE '%"FAILED"%'
), replaced AS (
	SELECT
		id, array_agg(REPLACE(element::text, 'FAILED', 'FAILED')::jsonb) AS new_status_history
	FROM (
		SELECT id, unnest(status_history) AS element
		FROM payments
	) AS subquery
	WHERE id IN (SELECT id FROM to_be_updated_cte)
	GROUP BY id
)
UPDATE
	payments
	SET status_history = replaced.new_status_history
FROM replaced
WHERE payments.id = replaced.id;
    

-- +migrate Down

-- Update the status_history column to replace `FAILED` with `FAILURE`
WITH to_be_updated_cte AS (
	SELECT
		DISTINCT payments.id
	FROM
		payments, unnest(payments.status_history) AS status_element
	WHERE
		status_element::text LIKE '%"FAILED"%'
), replaced AS (
	SELECT
		id, array_agg(REPLACE(element::text, 'FAILED', 'FAILURE')::jsonb) AS new_status_history
	FROM (
		SELECT id, unnest(status_history) AS element
		FROM payments
	) AS subquery
	WHERE id IN (SELECT id FROM to_be_updated_cte)
	GROUP BY id
)
UPDATE
	payments
	SET status_history = replaced.new_status_history
FROM replaced
WHERE payments.id = replaced.id;

-- Add back the unused stellar_main_address column
ALTER TABLE public.organizations
    ADD COLUMN stellar_main_address VARCHAR(56),
    ADD COLUMN are_payments_enabled BOOLEAN NOT NULL DEFAULT FALSE;
