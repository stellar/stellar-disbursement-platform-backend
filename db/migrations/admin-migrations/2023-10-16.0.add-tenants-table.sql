-- +migrate Up

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" SCHEMA public;
CREATE EXTENSION IF NOT EXISTS pgcrypto SCHEMA public;

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION update_at_refresh()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';
-- +migrate StatementEnd

CREATE TYPE tenant_status AS ENUM ('TENANT_CREATED', 'TENANT_PROVISIONED', 'TENANT_ACTIVATED', 'TENANT_DEACTIVATED');

CREATE TABLE tenants
(
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    name text NOT NULL,
    cors_allowed_origins text[] NULL,
    base_url text NULL,
    sdp_ui_base_url text NULL,
    status tenant_status DEFAULT 'TENANT_CREATED',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_unique_name ON tenants (LOWER(name));
CREATE TRIGGER refresh_tenants_updated_at BEFORE UPDATE ON tenants FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

COMMENT ON COLUMN tenants.base_url IS 'The SDP backend server''s base URL';
COMMENT ON COLUMN tenants.sdp_ui_base_url IS 'The SDP UI/dashboard Base URL.';

-- +migrate Down

DROP TABLE tenants;

DROP TYPE tenant_status;
