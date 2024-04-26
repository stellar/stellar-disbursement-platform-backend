-- +migrate Up

ALTER TABLE organizations
    ADD COLUMN is_approval_required boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN organizations.is_approval_required
    IS 'Column used to enable disbursement approval for organizations, requiring multiple users to start a disbursement.';

-- +migrate Down

ALTER TABLE organizations
    DROP COLUMN is_approval_required;
