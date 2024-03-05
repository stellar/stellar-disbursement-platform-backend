-- +migrate Up
ALTER TABLE public.tenants
    DROP COLUMN enable_mfa,
    DROP COLUMN enable_recaptcha;

-- +migrate Down
ALTER TABLE public.tenants
    ADD COLUMN enable_mfa boolean DEFAULT true,
    ADD COLUMN enable_recaptcha boolean DEFAULT true;
