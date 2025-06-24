-- Migration to prepare the payments table for direct payments

-- +migrate Up
-- 1. Add a new column to the payments table to distinguish between disbursement payments and direct payments
CREATE TYPE payment_type AS ENUM ('DISBURSEMENT', 'DIRECT');

ALTER TABLE payments
ADD COLUMN type payment_type NOT NULL DEFAULT 'DISBURSEMENT';

-- 2. Make disbursement_id nullable in the payments table to allow for direct payments
ALTER TABLE payments
    ALTER COLUMN disbursement_id DROP NOT NULL;

-- 3. Add a constraint to ensure that disbursement_id is only set for disbursement payments
ALTER TABLE payments
    ADD CONSTRAINT check_payment_type_disbursement
        CHECK (
            (type = 'DISBURSEMENT' AND disbursement_id IS NOT NULL) OR
            (type = 'DIRECT' AND disbursement_id IS NULL)
            );

-- 4. Create indexes
CREATE INDEX idx_payments_payment_type ON payments (type);
CREATE INDEX idx_payments_direct ON payments(id) WHERE type = 'DIRECT';
CREATE INDEX idx_payments_disbursement ON payments(disbursement_id) WHERE type = 'DISBURSEMENT';

-- +migrate Down

-- 1. Drop the indexes created in the Up migration
DROP INDEX IF EXISTS idx_payments_payment_type;
DROP INDEX IF EXISTS idx_payments_direct;
DROP INDEX IF EXISTS idx_payments_disbursement;

-- 2. Drop the constraint that checks the payment type and disbursement_id
ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS check_payment_type_disbursement;

-- 3. Make disbursement_id NOT NULL again. You may need to handle existing data before this step.
ALTER TABLE payments
    ALTER COLUMN disbursement_id SET NOT NULL;

-- 4. Drop the payment_type column and the payment_type enum type
ALTER TABLE payments DROP COLUMN type;

DROP TYPE IF EXISTS payment_type;
