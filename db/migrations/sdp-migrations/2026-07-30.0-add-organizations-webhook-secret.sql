-- Adds a per-tenant secret used to HMAC-sign outbox event deliveries (see event_delivery_job.go
-- deliver()), so consumers can verify a payload actually came from SDP and wasn't forged by
-- someone who merely learned the webhook URL. Auto-generated (pgcrypto, already enabled by
-- 2023-04-17.0-create-receiver_verifications-table.sql) rather than user-supplied, to avoid weak
-- secrets; never exposed via the API (see internal/data/organizations.go Organization.WebhookSecret).

-- +migrate Up
-- pgcrypto lands in the public schema (see 2023-04-17.0-create-receiver_verifications-table.sql);
-- tenant connections run with search_path scoped to their own schema only, so gen_random_bytes
-- must be schema-qualified here exactly like public.uuid_generate_v4() is elsewhere.
ALTER TABLE organizations ADD COLUMN webhook_secret TEXT NOT NULL DEFAULT encode(public.gen_random_bytes(32), 'hex');

-- +migrate Down
ALTER TABLE organizations DROP COLUMN webhook_secret;
