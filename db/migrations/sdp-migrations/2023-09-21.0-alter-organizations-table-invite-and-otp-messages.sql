-- +migrate Up

ALTER TABLE organizations
    DROP CONSTRAINT organization_sms_registration_message_template_contains_tags_check,
    ADD COLUMN otp_message_template VARCHAR(255) NOT NULL DEFAULT '{{.OTP}} is your {{.OrganizationName}} phone verification code.';

-- +migrate Down

ALTER TABLE organizations
    DROP COLUMN otp_message_template,
    ADD CONSTRAINT organization_sms_registration_message_template_contains_tags_check CHECK (sms_registration_message_template LIKE '%{{.OrganizationName}}%' AND sms_registration_message_template LIKE '%{{.RegistrationLink}}%');
