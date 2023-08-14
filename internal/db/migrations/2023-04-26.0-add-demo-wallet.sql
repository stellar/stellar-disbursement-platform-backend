-- This migration creates the sep_10_client_domain column in the public.wallets table and inserts the demo-wallet in the DB.

-- +migrate Up

ALTER TABLE public.wallets ADD COLUMN sep_10_client_domain VARCHAR(255) DEFAULT '' NOT NULL;

UPDATE public.wallets SET sep_10_client_domain = substring(homepage from 'https?://([^/]+)');
UPDATE public.wallets SET sep_10_client_domain = 'api-dev.vibrantapp.com' WHERE name = 'Vibrant Assist';
ALTER TABLE public.wallets ALTER COLUMN deep_link_schema TYPE VARCHAR(255);

-- +migrate Down
ALTER TABLE public.wallets DROP COLUMN sep_10_client_domain;
