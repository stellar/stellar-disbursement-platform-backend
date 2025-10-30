-- +migrate Up

ALTER TABLE organizations
ADD COLUMN mfa_disabled BOOLEAN,
ADD COLUMN captcha_disabled BOOLEAN;

COMMENT ON COLUMN organizations.mfa_disabled IS 'Organization-level MFA setting.';

COMMENT ON COLUMN organizations.captcha_disabled IS 'Organization-level CAPTCHA setting.';

-- +migrate Down

ALTER TABLE organizations
DROP COLUMN IF EXISTS mfa_disabled,
DROP COLUMN IF EXISTS captcha_disabled;