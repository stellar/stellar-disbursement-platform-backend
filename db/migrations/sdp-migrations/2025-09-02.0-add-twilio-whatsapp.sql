-- This is to add `TWILIO_WHATSAPP` to the `message_type` enum.

-- +migrate Up
ALTER TYPE message_type ADD VALUE 'TWILIO_WHATSAPP';


-- +migrate Down
-- Delete records using the enum value we want to remove
DELETE FROM messages WHERE type = 'TWILIO_WHATSAPP';

-- Replace the enum with a new definition that excludes `TWILIO_WHATSAPP`
CREATE TYPE message_type_new AS ENUM (
    'TWILIO_SMS',
    'AWS_SMS',
    'AWS_EMAIL',
    'DRY_RUN',
    'TWILIO_EMAIL'
    );

ALTER TABLE messages
    ALTER COLUMN type TYPE message_type_new
        USING type::text::message_type_new;

DROP TYPE message_type;
ALTER TYPE message_type_new RENAME TO message_type;