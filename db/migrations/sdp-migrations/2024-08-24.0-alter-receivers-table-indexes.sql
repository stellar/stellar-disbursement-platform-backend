-- +migrate Up
-- Remove existing unique constraint on phone_number
ALTER TABLE receivers DROP CONSTRAINT IF EXISTS payments_account_phone_number_key;

DROP INDEX IF EXISTS payments_account_phone_number_key;

-- Make phone_number nullable
ALTER TABLE receivers ALTER COLUMN phone_number DROP NOT NULL;

-- Add check constraint to ensure at least one contact method is provided
ALTER TABLE receivers ADD CONSTRAINT receiver_contact_check
    CHECK (phone_number IS NOT NULL OR email IS NOT NULL);

-- Create a unique index on both phone_number and email
CREATE UNIQUE INDEX receiver_unique_contact ON receivers (
    LOWER(COALESCE(phone_number, '')),
    LOWER(COALESCE(email, ''))
);

-- +migrate Down
-- Remove the new check constraint
ALTER TABLE receivers DROP CONSTRAINT IF EXISTS receiver_contact_check;

-- Remove the new unique index
DROP INDEX IF EXISTS receiver_unique_contact;

-- Restore phone_number to NOT NULL
UPDATE receivers SET phone_number = 'UNKNOWN' WHERE phone_number IS NULL;
ALTER TABLE receivers ALTER COLUMN phone_number SET NOT NULL;

-- Recreate the original unique constraint and index on phone_number
ALTER TABLE receivers ADD CONSTRAINT payments_account_phone_number_key UNIQUE (phone_number);