-- This migration adds the message_channel type and the message_channel_priority column to the organizations table.
-- +migrate Up

-- Create the message_channel enum type
CREATE TYPE message_channel AS ENUM ('SMS', 'EMAIL');

-- Add the message_channel_priority column to the organizations table
ALTER TABLE organizations
    ADD COLUMN message_channel_priority message_channel[] NOT NULL
        DEFAULT ARRAY['SMS'::message_channel, 'EMAIL'::message_channel];

-- Create a function to check if all message_channel values are included and valid
-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION check_message_channel_priority()
    RETURNS TRIGGER AS $$
DECLARE
    all_channels message_channel[];
    duplicate_channels message_channel[];
BEGIN
    -- Get all possible message_channel values
    SELECT array_agg(enumlabel::message_channel)
    INTO all_channels
    FROM pg_enum
    WHERE enumtypid = 'message_channel'::regtype;

    -- Check if all channels are included in the new value
    IF NOT (SELECT all_channels <@ NEW.message_channel_priority) THEN
        RAISE EXCEPTION 'message_channel_priority must include all possible message_channel values';
    END IF;

    -- Check for duplicates
    SELECT ARRAY(SELECT channel
                 FROM unnest(NEW.message_channel_priority) channel
                 GROUP BY channel
                 HAVING COUNT(*) > 1)
    INTO duplicate_channels;

    IF array_length(duplicate_channels, 1) > 0 THEN
        RAISE EXCEPTION 'message_channel_priority must not contain duplicate values: %', duplicate_channels;
    END IF;

    RETURN NEW;
END;
$$ language 'plpgsql';
-- +migrate StatementEnd

-- Create the trigger
CREATE TRIGGER validate_organizations_message_channel_priority
    BEFORE INSERT OR UPDATE OF message_channel_priority ON organizations
    FOR EACH ROW EXECUTE FUNCTION check_message_channel_priority();

-- +migrate Down

-- Drop the trigger
DROP TRIGGER IF EXISTS validate_organizations_message_channel_priority ON organizations;

-- Drop the function
DROP FUNCTION IF EXISTS check_message_channel_priority();

-- Remove the message_channel_priority column
ALTER TABLE organizations DROP COLUMN IF EXISTS message_channel_priority;

-- Remove the message_channel enum type
DROP TYPE IF EXISTS message_channel;