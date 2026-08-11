-- Create the distribution_wallets table: first-class multi-wallet entity owning its own
-- distribution account type, address, and lifecycle. Lives in the per-tenant schema, so the
-- (tenant, wallet) key from the spec is satisfied structurally (schema = tenant boundary).
-- NOTE: this is the tenant's SENDING account ("distribution wallet"), unrelated to the existing
-- `wallets` table which models recipient wallet providers.

-- +migrate Up

CREATE TABLE distribution_wallets (
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    name VARCHAR(64) NOT NULL,
    description TEXT,
    distribution_account_address VARCHAR(56),
    distribution_account_type VARCHAR(64) NOT NULL
        CHECK (distribution_account_type IN (
            'DISTRIBUTION_ACCOUNT.STELLAR.ENV',
            'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT',
            'DISTRIBUTION_ACCOUNT.CIRCLE.DB_VAULT'
        )),
    distribution_account_status VARCHAR(36) NOT NULL DEFAULT 'ACTIVE'
        CHECK (distribution_account_status IN ('ACTIVE', 'PENDING_USER_ACTIVATION')),
    status VARCHAR(16) NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ,
    -- The default wallet must always be an active wallet (DB-layer piece of the
    -- zero-active-wallets invariant; also enforced at the API layer).
    CONSTRAINT distribution_wallets_default_must_be_active
        CHECK (NOT (is_default AND status = 'ARCHIVED')),
    -- archived_at is set if and only if the wallet is archived.
    CONSTRAINT distribution_wallets_archived_at_consistency
        CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL))
);

-- Wallet names are unique within a tenant (schema = tenant).
CREATE UNIQUE INDEX distribution_wallets_name_unique_idx ON distribution_wallets (name);

-- At most ONE default wallet per tenant.
CREATE UNIQUE INDEX distribution_wallets_single_default_idx ON distribution_wallets ((TRUE)) WHERE is_default;

-- A distribution account address can back at most one wallet (NULL allowed while a
-- CIRCLE wallet is pending user activation).
CREATE UNIQUE INDEX distribution_wallets_address_unique_idx
    ON distribution_wallets (distribution_account_address)
    WHERE distribution_account_address IS NOT NULL;

-- TRIGGER: updated_at
CREATE TRIGGER refresh_distribution_wallets_updated_at
    BEFORE UPDATE ON distribution_wallets
    FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- Immutable history for audit, dispute, and compliance lookups.
SELECT create_audit_table('distribution_wallets');

-- +migrate Down

SELECT drop_audit_table('distribution_wallets');

DROP TRIGGER refresh_distribution_wallets_updated_at ON distribution_wallets;

DROP TABLE distribution_wallets;
