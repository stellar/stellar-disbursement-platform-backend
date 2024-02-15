-- +migrate Up
ALTER TABLE
    disbursements
ADD
    COLUMN sms_registration_message_template TEXT NULL;

-- +migrate Down
ALTER TABLE
    disbursements DROP COLUMN sms_registration_message_template;