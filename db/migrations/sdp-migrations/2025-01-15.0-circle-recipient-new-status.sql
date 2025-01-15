-- Add a new status 'failed' to the circle_recipient_status enum type

-- +migrate Up
ALTER TYPE circle_recipient_status ADD VALUE 'failed';


-- +migrate Down
-- Update any existing records with 'failed' status to 'denied'
UPDATE circle_recipients SET status = 'denied' WHERE status = 'failed';

-- Rename the existing enum type to a temporary name
ALTER TYPE circle_recipient_status RENAME TO circle_recipient_status_old;

-- Create a new enum type without the 'failed' value
CREATE TYPE circle_recipient_status AS ENUM ('pending', 'active', 'inactive', 'denied');

-- Update all references of the old type to the new type
ALTER TABLE circle_recipients
    ALTER COLUMN status TYPE circle_recipient_status USING status::text::circle_recipient_status;

-- Drop the old enum type
DROP TYPE circle_recipient_status_old;
