-- +migrate Up
-- Rename organizations.sms_resend_interval to organizations.receiver_invitation_resend_interval
ALTER TABLE organizations
    RENAME COLUMN sms_resend_interval TO receiver_invitation_resend_interval_days;

-- Rename organizations.sms_registration_message_template to organizations.receiver_registration_message_template
ALTER TABLE organizations
    RENAME COLUMN sms_registration_message_template TO receiver_registration_message_template;

-- Update the constraint name for resend interval
ALTER TABLE organizations
    DROP CONSTRAINT organization_sms_resend_interval_valid_value_check;

ALTER TABLE organizations
    ADD CONSTRAINT organization_invitation_resend_interval_valid_value_check
        CHECK ((receiver_invitation_resend_interval_days IS NOT NULL AND receiver_invitation_resend_interval_days > 0) OR receiver_invitation_resend_interval_days IS NULL);

-- Rename disbursements.sms_registration_message_template to disbursements.receiver_registration_message_template
ALTER TABLE disbursements
    RENAME COLUMN sms_registration_message_template TO receiver_registration_message_template;


-- +migrate Down
ALTER TABLE organizations
    RENAME COLUMN receiver_invitation_resend_interval_days TO sms_resend_interval;

ALTER TABLE organizations
    RENAME COLUMN receiver_registration_message_template TO sms_registration_message_template;

ALTER TABLE organizations
    DROP CONSTRAINT organization_invitation_resend_interval_valid_value_check;

ALTER TABLE organizations
    ADD CONSTRAINT organization_sms_resend_interval_valid_value_check
        CHECK ((sms_resend_interval IS NOT NULL AND sms_resend_interval > 0) OR sms_resend_interval IS NULL);

ALTER TABLE disbursements
    RENAME COLUMN receiver_registration_message_template TO sms_registration_message_template;
