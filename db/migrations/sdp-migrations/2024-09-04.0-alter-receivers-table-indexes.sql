-- +migrate Up
-- Remove existing unique constraint on phone_number
ALTER TABLE receivers DROP CONSTRAINT IF EXISTS payments_account_phone_number_key;

DROP INDEX IF EXISTS payments_account_phone_number_key;

-- Make phone_number nullable
ALTER TABLE receivers ALTER COLUMN phone_number DROP NOT NULL;

-- Add check constraint to ensure at least one contact method is provided
ALTER TABLE receivers ADD CONSTRAINT receiver_contact_check
    CHECK (phone_number IS NOT NULL OR email IS NOT NULL);

-- Create unique indexes on both phone_number and email separately
CREATE UNIQUE INDEX receiver_unique_phone_number ON receivers (LOWER(phone_number))
    WHERE phone_number IS NOT NULL;

CREATE UNIQUE INDEX receiver_unique_email ON receivers (LOWER(email))
    WHERE email IS NOT NULL;

-- +migrate Down
-- Remove the new check constraint
ALTER TABLE receivers DROP CONSTRAINT IF EXISTS receiver_contact_check;

-- Remove the new unique indexes
DROP INDEX IF EXISTS receiver_unique_phone_number;
DROP INDEX IF EXISTS receiver_unique_email;

-- Restore phone_number to NOT NULL
UPDATE receivers SET phone_number = 'UNKNOWN' WHERE phone_number IS NULL;
ALTER TABLE receivers ALTER COLUMN phone_number SET NOT NULL;

-- Recreate the original unique constraint and index on phone_number
ALTER TABLE receivers ADD CONSTRAINT payments_account_phone_number_key UNIQUE (phone_number);