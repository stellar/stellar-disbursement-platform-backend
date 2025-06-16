-- +migrate Up
CREATE TYPE payment_type AS ENUM ('DISBURSEMENT', 'DIRECT');

ALTER TABLE payments
ADD COLUMN payment_type payment_type NOT NULL DEFAULT 'DISBURSEMENT';

CREATE INDEX idx_payments_payment_type ON payments (payment_type);

-- +migrate Down
DROP INDEX IF EXISTS idx_payments_payment_type;

ALTER TABLE payments DROP COLUMN payment_type;
DROP TYPE IF EXISTS payment_type;
