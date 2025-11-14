-- +migrate Up
-- This migration creates the sep24_transactions table for tracking SEP-24 transaction IDs.
-- This table is created per-tenant and is used to ensure SEP-24 compliance by tracking
-- transaction IDs created by SDP. Only transactions stored in this table can be queried
-- via the GET /transaction endpoint.
--
-- For new tenants: This migration is automatically applied during tenant provisioning.
-- For existing tenants: Run `stellar-disbursement-platform db sdp migrate up` to apply this migration.

CREATE TABLE sep24_transactions (
    id VARCHAR(36) PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sep24_transactions_created_at ON sep24_transactions (created_at);

-- +migrate Down

DROP INDEX IF EXISTS idx_sep24_transactions_created_at;
DROP TABLE sep24_transactions;

