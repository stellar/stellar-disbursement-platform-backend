-- +migrate Up

ALTER TABLE public.organizations
    ADD COLUMN is_approval_required boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN public.organizations.is_approval_required
    IS 'Column used to enable disbursement approval for organizations, requiring multiple users to start a disbursement.';

-- +migrate Down

ALTER TABLE public.organizations
    DROP COLUMN is_approval_required;
