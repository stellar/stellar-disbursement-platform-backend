-- This migration adds the 'SEP24_REGISTRATION' value to the verification_type enum.
-- +migrate Up
ALTER TYPE verification_type ADD VALUE 'SEP24_REGISTRATION';

-- +migrate Down
-- Create a replacement enum without the embedded wallet registration value
CREATE TYPE verification_type_new AS ENUM ('DATE_OF_BIRTH', 'PIN', 'NATIONAL_ID_NUMBER', 'YEAR_MONTH');

-- Add columns with the replacement enum type
ALTER TABLE receiver_verifications ADD COLUMN verification_field_new verification_type_new;
ALTER TABLE receiver_verifications_audit ADD COLUMN verification_field_new verification_type_new;
ALTER TABLE disbursements ADD COLUMN verification_field_new verification_type_new;

-- Copy and transform existing data
UPDATE receiver_verifications SET verification_field_new =
CASE
    WHEN verification_field = 'SEP24_REGISTRATION' THEN 'DATE_OF_BIRTH'::verification_type_new
    ELSE verification_field::text::verification_type_new
END;
UPDATE receiver_verifications_audit SET verification_field_new =
CASE
    WHEN verification_field = 'SEP24_REGISTRATION' THEN 'DATE_OF_BIRTH'::verification_type_new
    ELSE verification_field::text::verification_type_new
END;
UPDATE disbursements SET verification_field_new =
CASE
    WHEN verification_field = 'SEP24_REGISTRATION' THEN 'DATE_OF_BIRTH'::verification_type_new
    ELSE verification_field::text::verification_type_new
END;

-- Drop the old columns
ALTER TABLE receiver_verifications DROP COLUMN verification_field;
ALTER TABLE receiver_verifications_audit DROP COLUMN verification_field;
ALTER TABLE disbursements DROP COLUMN verification_field;

-- Rename the new columns back to the original name
ALTER TABLE receiver_verifications RENAME COLUMN verification_field_new TO verification_field;
ALTER TABLE receiver_verifications_audit RENAME COLUMN verification_field_new TO verification_field;
ALTER TABLE disbursements RENAME COLUMN verification_field_new TO verification_field;

-- Drop the original enum type and rename the new one
DROP TYPE verification_type;
ALTER TYPE verification_type_new RENAME TO verification_type;
