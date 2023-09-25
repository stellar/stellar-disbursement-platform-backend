-- This is to update payment_status to change `FAILURE` to `FAILED`.

-- +migrate Up

ALTER TYPE payment_status RENAME VALUE 'FAILURE' TO 'FAILED';

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

ALTER TYPE payment_status RENAME VALUE 'FAILED' TO 'FAILURE';