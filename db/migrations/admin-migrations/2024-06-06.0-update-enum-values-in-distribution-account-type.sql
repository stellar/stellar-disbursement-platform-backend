-- +migrate Up

-- Remove default value, and change type to text
ALTER TABLE tenants 
    ALTER COLUMN distribution_account_type DROP DEFAULT,
    ALTER COLUMN distribution_account_type TYPE text;

-- Drop enum
DROP TYPE distribution_account_type;

-- Update values of the text column
UPDATE tenants SET distribution_account_type = 'DISTRIBUTION_ACCOUNT.STELLAR.ENV' WHERE distribution_account_type = 'ENV_STELLAR';
UPDATE tenants SET distribution_account_type = 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT' WHERE distribution_account_type = 'DB_VAULT_STELLAR';
UPDATE tenants SET distribution_account_type = 'DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT' WHERE distribution_account_type = 'DB_VAULT_CIRCLE';

-- Create a new enum
CREATE TYPE distribution_account_type AS ENUM (
    'DISTRIBUTION_ACCOUNT.STELLAR.ENV',
    'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT',
    'DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT'
);

-- Update column to new enum type, and set default value
ALTER TABLE tenants 
    ALTER COLUMN distribution_account_type TYPE distribution_account_type USING distribution_account_type::text::distribution_account_type,
    ALTER COLUMN distribution_account_type SET DEFAULT 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT';


-- +migrate Down

-- Remove default value, and change type to text
ALTER TABLE tenants 
    ALTER COLUMN distribution_account_type DROP DEFAULT,
    ALTER COLUMN distribution_account_type TYPE text;

-- Drop enum
DROP TYPE distribution_account_type;

-- Update values of the text column
UPDATE tenants SET distribution_account_type = 'ENV_STELLAR' WHERE distribution_account_type = 'DISTRIBUTION_ACCOUNT.STELLAR.ENV';
UPDATE tenants SET distribution_account_type = 'DB_VAULT_STELLAR' WHERE distribution_account_type = 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT';
UPDATE tenants SET distribution_account_type = 'DB_VAULT_CIRCLE' WHERE distribution_account_type = 'DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT';

-- Create a new enum
CREATE TYPE distribution_account_type AS ENUM (
    'ENV_STELLAR',
    'DB_VAULT_STELLAR',
    'DB_VAULT_CIRCLE'
);

-- Update column to new enum type, and set default value
ALTER TABLE tenants 
    ALTER COLUMN distribution_account_type TYPE distribution_account_type USING distribution_account_type::text::distribution_account_type,
    ALTER COLUMN distribution_account_type SET DEFAULT 'DB_VAULT_STELLAR';
