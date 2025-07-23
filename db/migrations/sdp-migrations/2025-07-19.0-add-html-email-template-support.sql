-- +migrate Up

-- Expand messages.text_encrypted to support larger HTML email content
ALTER TABLE messages ALTER COLUMN text_encrypted TYPE TEXT;

ALTER TABLE organizations
    ADD COLUMN receiver_registration_html_email_template TEXT;

ALTER TABLE organizations 
    ADD COLUMN receiver_registration_html_email_subject TEXT;

-- +migrate Down

ALTER TABLE organizations DROP COLUMN IF EXISTS receiver_registration_html_email_subject;
ALTER TABLE organizations DROP COLUMN IF EXISTS receiver_registration_html_email_template;

ALTER TABLE messages ALTER COLUMN text_encrypted TYPE VARCHAR(1024);