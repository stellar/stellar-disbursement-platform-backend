-- +migrate Up

ALTER TABLE public.auth_users ADD COLUMN is_active boolean DEFAULT true;

-- +migrate Down

ALTER TABLE public.auth_users DROP COLUMN is_active;
