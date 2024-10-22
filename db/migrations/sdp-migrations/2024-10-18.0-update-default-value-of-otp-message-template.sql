-- +migrate Up

ALTER TABLE organizations
    ALTER COLUMN otp_message_template SET DEFAULT '{{.OTP}} is your {{.OrganizationName}} verification code.';

UPDATE organizations
    SET otp_message_template = '{{.OTP}} is your {{.OrganizationName}} verification code.'
    WHERE otp_message_template = '{{.OTP}} is your {{.OrganizationName}} phone verification code.';

-- +migrate Down

ALTER TABLE organizations
    ALTER COLUMN otp_message_template SET DEFAULT '{{.OTP}} is your {{.OrganizationName}} phone verification code.';

UPDATE organizations
    SET otp_message_template = '{{.OTP}} is your {{.OrganizationName}} phone verification code.'
    WHERE otp_message_template = '{{.OTP}} is your {{.OrganizationName}} verification code.';
