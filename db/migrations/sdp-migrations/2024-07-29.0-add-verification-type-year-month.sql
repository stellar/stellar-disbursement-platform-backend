-- This migration updates the verification_field column in the receiver_verifications table to include a new verification type called 'YEAR_MONTH'.
-- +migrate Up
ALTER TYPE verification_type ADD VALUE 'YEAR_MONTH';

-- +migrate Down
-- Create new type
CREATE TYPE verification_type_new AS ENUM ('DATE_OF_BIRTH', 'PIN', 'NATIONAL_ID_NUMBER');

-- Add a new column with the new type (receiver_verifications & disbursements)
ALTER TABLE receiver_verifications ADD COLUMN verification_field_new verification_type_new;
ALTER TABLE disbursements ADD COLUMN verification_field_new verification_type_new;

-- Copy & transform data (receiver_verifications & disbursements)
UPDATE receiver_verifications SET verification_field_new = 
CASE 
    WHEN verification_field = 'YEAR_MONTH' THEN 'DATE_OF_BIRTH'::verification_type_new
    ELSE verification_field::text::verification_type_new
END;
UPDATE disbursements SET verification_field_new = 
CASE 
    WHEN verification_field = 'YEAR_MONTH' THEN 'DATE_OF_BIRTH'::verification_type_new
    ELSE verification_field::text::verification_type_new
END;

-- Drop the old column (receiver_verifications & disbursements)
ALTER TABLE receiver_verifications DROP COLUMN verification_field;
ALTER TABLE disbursements DROP COLUMN verification_field;

-- Rename the new column (receiver_verifications & disbursements)
ALTER TABLE receiver_verifications RENAME COLUMN verification_field_new TO verification_field;
ALTER TABLE disbursements RENAME COLUMN verification_field_new TO verification_field;

-- Drop old type
DROP TYPE verification_type;

-- Rename new type
ALTER TYPE verification_type_new RENAME TO verification_type;
