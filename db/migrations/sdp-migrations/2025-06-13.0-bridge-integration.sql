-- Adds support for Bridge integration

-- +migrate Up
-- Add bridge_integration_status enum.
CREATE TYPE bridge_integration_status AS ENUM (
    'NOT_ENABLED',
    'NOT_OPTED_IN',
    'OPTED_IN',
    'READY_FOR_DEPOSIT',
    'ERROR'
);

-- Add new bridge_integration table.
CREATE TABLE bridge_integration (
    status bridge_integration_status NOT NULL DEFAULT 'OPTED_IN',
    kyc_link_id VARCHAR(36) NOT NULL, -- Bridge KYC link ID (NOT NULL)
    customer_id VARCHAR(36) NOT NULL, -- Bridge customer ID (NOT NULL)
    opted_in_by TEXT, -- Email of the user who opted in for Bridge integration (nullable)
    opted_in_at TIMESTAMPTZ, -- When user opted in (nullable)
    virtual_account_id VARCHAR(36), -- Bridge virtual account ID (nullable)
    virtual_account_created_at TIMESTAMPTZ, -- When the virtual account was created (nullable)
    virtual_account_created_by TEXT, -- Email of the user who created the virtual account (nullable)
    error_message TEXT, -- Error messages related to Bridge integration (nullable)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add updated_at trigger.
CREATE TRIGGER refresh_bridge_integration_updated_at BEFORE UPDATE ON bridge_integration FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- Add constraint checks for integration status consistency.
ALTER TABLE bridge_integration ADD CONSTRAINT bridge_integration_not_opted_in_check
    CHECK (
        status != 'NOT_OPTED_IN' OR
        (kyc_link_id IS NULL AND virtual_account_id IS NULL)
    );

ALTER TABLE bridge_integration ADD CONSTRAINT bridge_integration_opted_in_check
    CHECK (
        status != 'OPTED_IN' OR
        (kyc_link_id IS NOT NULL AND virtual_account_id IS NULL AND opted_in_by IS NOT NULL AND opted_in_at IS NOT NULL)
    );

ALTER TABLE bridge_integration ADD CONSTRAINT bridge_integration_ready_for_deposit_check 
    CHECK (
        status != 'READY_FOR_DEPOSIT' OR
        (kyc_link_id IS NOT NULL AND virtual_account_id IS NOT NULL AND virtual_account_created_by IS NOT NULL AND virtual_account_created_at IS NOT NULL)
    );

ALTER TABLE bridge_integration ADD CONSTRAINT bridge_integration_error_check 
    CHECK (
        status != 'ERROR' OR
        error_message IS NOT NULL
    );


-- Ensure only one Bridge integration per tenant.
CREATE UNIQUE INDEX idx_bridge_integration_singleton ON bridge_integration ((1));

-- +migrate Down
-- Drop indexes.
DROP INDEX IF EXISTS idx_bridge_integration_singleton;

-- Drop trigger.
DROP TRIGGER IF EXISTS refresh_bridge_integration_updated_at ON bridge_integration;

-- Drop table and types.
DROP TABLE IF EXISTS bridge_integration CASCADE;
DROP TYPE IF EXISTS bridge_integration_status;