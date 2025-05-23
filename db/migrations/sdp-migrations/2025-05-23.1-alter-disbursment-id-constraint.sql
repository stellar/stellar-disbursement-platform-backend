-- +migrate Up
-- +migrate StatementBegin
ALTER TABLE payments 
ALTER COLUMN disbursement_id DROP NOT NULL;

ALTER TABLE payments 
ADD CONSTRAINT check_payment_type_disbursement 
CHECK (
    (payment_type = 'DISBURSEMENT' AND disbursement_id IS NOT NULL) OR
    (payment_type = 'DIRECT' AND disbursement_id IS NULL)
);

CREATE INDEX idx_payments_direct ON payments(id) WHERE payment_type = 'DIRECT';
CREATE INDEX idx_payments_disbursement ON payments(disbursement_id) WHERE payment_type = 'DISBURSEMENT';

-- Add comment for documentation
COMMENT ON COLUMN payments.disbursement_id IS 'Reference to disbursement. NULL for direct payments, required for disbursement payments';
-- +migrate StatementEnd

-- +migrate Down
-- +migrate StatementBegin
DROP INDEX IF EXISTS idx_payments_direct;
DROP INDEX IF EXISTS idx_payments_disbursement;

ALTER TABLE payments 
DROP CONSTRAINT IF EXISTS check_payment_type_disbursement;

DELETE FROM payments WHERE disbursement_id IS NULL;

-- Make disbursement_id NOT NULL again
ALTER TABLE payments 
ALTER COLUMN disbursement_id SET NOT NULL;

-- Remove comment
COMMENT ON COLUMN payments.disbursement_id IS NULL;
-- +migrate StatementEnd