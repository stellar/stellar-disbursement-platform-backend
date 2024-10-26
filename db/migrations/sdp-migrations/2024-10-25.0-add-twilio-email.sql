-- This is to add `TWILIO_EMAIL` to the `message_type` enum.

-- +migrate Up
ALTER TYPE message_type ADD VALUE 'TWILIO_EMAIL';


-- +migrate Down
CREATE TYPE temp_message_type AS ENUM (
    'TWILIO_SMS',
    'AWS_SMS',
    'AWS_EMAIL',
    'DRY_RUN'
    );

DELETE FROM messages WHERE type = 'TWILIO_EMAIL';

ALTER TABLE messages
    ALTER COLUMN type TYPE temp_message_type USING type::text::temp_message_type;

DROP TYPE message_type;

ALTER TYPE temp_message_type RENAME TO message_type;