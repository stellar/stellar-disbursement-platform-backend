-- +migrate Up
ALTER TABLE receiver_verifications
    ADD COLUMN verification_channel message_channel;

UPDATE receiver_verifications rv
    SET verification_channel = 'SMS'::message_channel
    WHERE rv.verification_channel IS NULL AND confirmed_at IS NOT NULL;

-- +migrate Down

ALTER TABLE receiver_verifications
    DROP COLUMN verification_channel;