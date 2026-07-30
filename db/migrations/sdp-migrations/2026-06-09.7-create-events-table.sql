-- Transactional outbox for webhook/internal events: event rows are written in the SAME
-- transaction as the state change they describe, so an event exists iff the change committed.
-- A scheduler job delivers undelivered rows to the tenant's configured webhook URL
-- (at-least-once; consumers deduplicate on event id).

-- +migrate Up

CREATE TABLE events (
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    event_type VARCHAR(64) NOT NULL,
    schema_version INT NOT NULL DEFAULT 1,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    delivery_attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX events_undelivered_idx ON events (occurred_at) WHERE delivered_at IS NULL;

-- Tenant-configured webhook destination (Owner-managed); delivery is skipped when unset.
ALTER TABLE organizations ADD COLUMN webhook_url TEXT;

-- +migrate Down

ALTER TABLE organizations DROP COLUMN webhook_url;

DROP INDEX events_undelivered_idx;
DROP TABLE events;
