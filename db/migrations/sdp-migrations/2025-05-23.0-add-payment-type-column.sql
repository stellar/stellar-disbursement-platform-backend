-- +migrate Up
-- +migrate StatementBegin
ALTER TABLE payments 
ADD COLUMN payment_type VARCHAR(20) DEFAULT 'DISBURSEMENT' NOT NULL 
CHECK (payment_type IN ('DISBURSEMENT', 'DIRECT'));

UPDATE payments SET payment_type = 'DISBURSEMENT';

CREATE INDEX idx_payments_payment_type ON payments(payment_type);
-- +migrate StatementEnd

-- +migrate Down
-- +migrate StatementBegin
DROP INDEX IF EXISTS idx_payments_payment_type;

ALTER TABLE payments 
DROP COLUMN payment_type;
-- +migrate StatementEnd