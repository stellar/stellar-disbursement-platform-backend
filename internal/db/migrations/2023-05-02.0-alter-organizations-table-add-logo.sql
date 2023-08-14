-- +migrate Up

ALTER TABLE public.organizations
    ADD COLUMN logo BYTEA NULL;

-- +migrate Down

ALTER TABLE public.organizations
    DROP COLUMN logo;
