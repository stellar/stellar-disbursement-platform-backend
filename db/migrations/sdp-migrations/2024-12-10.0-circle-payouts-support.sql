-- prepares the database for Circle payouts by adding the new circle_recipients table, and modifying circle_transfers to be used for payouts.

-- +migrate Up
-- circle_recipients
CREATE TYPE circle_recipient_status AS ENUM ('pending', 'active', 'inactive', 'denied');

CREATE TABLE circle_recipients (
    receiver_wallet_id VARCHAR(36) PRIMARY KEY,
    idempotency_key VARCHAR(36) NOT NULL DEFAULT public.uuid_generate_v4(),
    circle_recipient_id VARCHAR(36),
    status circle_recipient_status,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sync_attempts INT NOT NULL DEFAULT 0,
    last_sync_attempt_at TIMESTAMPTZ,
    response_body JSONB,
    CONSTRAINT unique_idempotency_key UNIQUE (idempotency_key)
);

CREATE TRIGGER refresh_circle_recipient_updated_at BEFORE UPDATE ON circle_recipients FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- circle_transfer_requests
ALTER TABLE circle_transfer_requests
    ADD COLUMN circle_payout_id VARCHAR(36),
    ADD CONSTRAINT circle_transfer_or_payout CHECK (
        NOT (circle_transfer_id IS NOT NULL AND circle_payout_id IS NOT NULL)
    );

-- status indexes
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_disbursements_status ON disbursements(status);
CREATE INDEX idx_receiver_wallets_status ON receiver_wallets(status);

-- +migrate Down
-- status indexes
DROP INDEX IF EXISTS idx_payments_status;
DROP INDEX IF EXISTS idx_disbursements_status;
DROP INDEX IF EXISTS idx_receiver_wallets_status;

-- circle_transfer_requests
ALTER TABLE circle_transfer_requests
    DROP CONSTRAINT circle_transfer_or_payout,
    DROP COLUMN circle_payout_id CASCADE;

-- circle_recipients
DROP TRIGGER refresh_circle_recipient_updated_at ON circle_recipients;
DROP TABLE circle_recipients CASCADE;
DROP TYPE circle_recipient_status;
