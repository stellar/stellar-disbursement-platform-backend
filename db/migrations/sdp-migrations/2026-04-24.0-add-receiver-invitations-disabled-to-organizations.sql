-- +migrate Up

ALTER TABLE organizations
ADD COLUMN receiver_invitations_disabled BOOLEAN;

COMMENT ON COLUMN organizations.receiver_invitations_disabled
    IS 'When true, the scheduled receiver wallet invitation job sends no invites for this organization.';

-- +migrate Down

ALTER TABLE organizations
DROP COLUMN IF EXISTS receiver_invitations_disabled;
