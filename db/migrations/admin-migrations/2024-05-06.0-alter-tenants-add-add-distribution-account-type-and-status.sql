-- +migrate Up

CREATE TYPE distribution_account_type AS ENUM (
    'ENV_STELLAR',
    'DB_VAULT_STELLAR',
    'DB_VAULT_CIRCLE'
);
CREATE TYPE distribution_account_status AS ENUM (
    'ACTIVE',
    'PENDING_USER_ACTIVATION'
);

ALTER TABLE tenants
    ADD COLUMN distribution_account_type distribution_account_type NOT NULL DEFAULT 'DB_VAULT_STELLAR',
    ADD COLUMN distribution_account_status distribution_account_status NOT NULL DEFAULT 'ACTIVE';

ALTER TABLE tenants RENAME COLUMN distribution_account TO distribution_account_address;

-- +migrate Down
ALTER TABLE tenants RENAME COLUMN distribution_account_address TO distribution_account;

ALTER TABLE tenants
    DROP COLUMN distribution_account_type,
    DROP COLUMN distribution_account_status;

DROP TYPE distribution_account_type;
DROP TYPE distribution_account_status;
