-- +migrate Up

ALTER TABLE payments
    ADD COLUMN external_payment_id VARCHAR(64) NULL;

-- +migrate Down

ALTER TABLE payments
    DROP COLUMN external_payment_id;
