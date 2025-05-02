-- +migrate Up

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    key_hash VARCHAR(64) NOT NULL,
    salt VARCHAR(32) NOT NULL,
    permissions VARCHAR(32) [] NOT NULL,
    allowed_ips VARCHAR(64) [] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by UUID NOT NULL,
    expiry_date TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

-- enforce fast, unique lookups on the hash
CREATE UNIQUE INDEX IF NOT EXISTS api_keys_key_hash_idx ON api_keys (key_hash);

-- create the audit table/triggers
SELECT create_audit_table ('api_keys');

-- +migrate Down

-- drop the audit triggers/table first
SELECT drop_audit_table ('api_keys');

-- drop the table (automatically removes its indexes)
DROP TABLE IF EXISTS api_keys;