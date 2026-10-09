-- +migrate Up

ALTER TABLE organizations
ADD COLUMN reporting_enabled BOOLEAN DEFAULT FALSE;

COMMENT ON COLUMN organizations.reporting_enabled IS 'Organization-level flag to enable the Reports feature.';

-- +migrate Down

ALTER TABLE organizations
DROP COLUMN IF EXISTS reporting_enabled;
