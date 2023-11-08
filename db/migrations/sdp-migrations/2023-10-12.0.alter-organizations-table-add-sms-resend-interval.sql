-- +migrate Up

ALTER TABLE organizations
    ADD COLUMN sms_resend_interval INTEGER NULL,
    ADD CONSTRAINT organization_sms_resend_interval_valid_value_check CHECK ((sms_resend_interval IS NOT NULL AND sms_resend_interval > 0) OR sms_resend_interval IS NULL);

-- +migrate Down

ALTER TABLE organizations
    DROP CONSTRAINT organization_sms_resend_interval_valid_value_check,
    DROP COLUMN sms_resend_interval;
