-- Drop unused cors allowed origins column.

-- +migrate Up

ALTER TABLE tenants
    DROP COLUMN IF EXISTS cors_allowed_origins;


-- +migrate Down

ALTER TABLE tenants
    ADD COLUMN cors_allowed_origins text[] NULL;
