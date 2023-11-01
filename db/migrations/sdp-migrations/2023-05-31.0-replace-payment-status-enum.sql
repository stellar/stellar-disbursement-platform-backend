-- This is to update payment_status to change `FAILURE` to `FAILED`.

-- +migrate Up

ALTER TYPE payment_status RENAME VALUE 'FAILURE' TO 'FAILED';

-- +migrate Down

ALTER TYPE payment_status RENAME VALUE 'FAILED' TO 'FAILURE';