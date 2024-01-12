-- +migrate Up
ALTER TABLE
    public.disbursements
ADD
    COLUMN sms_registration_message_template TEXT NULL;

-- +migrate Down
ALTER TABLE
    public.disbursements DROP COLUMN sms_registration_message_template;