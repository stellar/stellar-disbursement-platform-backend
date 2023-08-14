-- This is to add `DRY_RUN` to the `message_type` enum.

-- +migrate Up
ALTER TYPE message_type ADD VALUE 'DRY_RUN';


-- +migrate Down
CREATE TYPE temp_message_type AS ENUM (
    'TWILIO_SMS',
    'AWS_SMS',
    'AWS_EMAIL'
);

DELETE FROM messages WHERE type = 'DRY_RUN';

ALTER TABLE messages
    ALTER COLUMN type TYPE temp_message_type USING type::text::temp_message_type;

DROP TYPE message_type;

ALTER TYPE temp_message_type RENAME TO message_type;