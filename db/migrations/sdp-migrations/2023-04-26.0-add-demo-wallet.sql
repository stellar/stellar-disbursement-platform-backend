-- This migration creates the sep_10_client_domain column in the wallets table and inserts the demo-wallet in the DB.

-- +migrate Up

ALTER TABLE wallets ADD COLUMN sep_10_client_domain VARCHAR(255) DEFAULT '' NOT NULL;

UPDATE wallets SET sep_10_client_domain = substring(homepage from 'https?://([^/]+)');
UPDATE wallets SET sep_10_client_domain = 'api-dev.vibrantapp.com' WHERE name = 'Vibrant Assist';
ALTER TABLE wallets ALTER COLUMN deep_link_schema TYPE VARCHAR(255);

-- +migrate Down
ALTER TABLE wallets DROP COLUMN sep_10_client_domain;
