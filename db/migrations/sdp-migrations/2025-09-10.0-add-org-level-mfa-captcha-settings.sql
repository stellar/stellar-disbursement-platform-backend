-- +migrate Up

ALTER TABLE organizations
    ADD COLUMN mfa_enabled BOOLEAN DEFAULT NULL,
    ADD COLUMN captcha_enabled BOOLEAN DEFAULT NULL;

COMMENT ON COLUMN organizations.mfa_enabled IS 'Organization-level MFA setting. NULL means use environment default (DISABLE_MFA flag)';
COMMENT ON COLUMN organizations.captcha_enabled IS 'Organization-level CAPTCHA setting. NULL means use environment default (DISABLE_RECAPTCHA flag)';

-- +migrate Down

ALTER TABLE organizations
    DROP COLUMN IF EXISTS mfa_enabled,
    DROP COLUMN IF EXISTS captcha_enabled;