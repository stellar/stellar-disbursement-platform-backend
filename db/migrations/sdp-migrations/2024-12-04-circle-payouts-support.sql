-- prepares the database for Circle payouts by adding the new circle_recipients table, and modifying circle_transfers to be used for payouts.

-- +migrate Up
CREATE TYPE circle_recipient_status AS ENUM ('pending', 'complete', 'failed');

CREATE TABLE circle_recipients (
    receiver_wallet_id VARCHAR(36) PRIMARY KEY CONSTRAINT fk_circle_recipient_receiver_wallet_id REFERENCES receiver_wallets(id),
    idempotency_key VARCHAR(36) NOT NULL DEFAULT public.uuid_generate_v4(),
    circle_recipient_id VARCHAR(36) NOT NULL,
    status circle_recipient_status,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sync_attempts INT NOT NULL DEFAULT 0,
    last_sync_attempt_at TIMESTAMPTZ,
    CONSTRAINT unique_idempotency_key UNIQUE (idempotency_key)
);

ALTER TABLE circle_transfer_requests
    ADD COLUMN circle_payout_id VARCHAR(36),
    ADD CONSTRAINT circle_transfer_or_payout CHECK (
        NOT (circle_transfer_id IS NOT NULL AND circle_payout_id IS NOT NULL)
    );

-- TRIGGER: updated_at
CREATE TRIGGER refresh_circle_recipient_updated_at BEFORE UPDATE ON circle_recipients FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- +migrate Down
ALTER TABLE circle_transfer_requests
    DROP CONSTRAINT circle_transfer_or_payout,
    DROP COLUMN circle_payout_id CASCADE;

DROP TRIGGER refresh_circle_recipient_updated_at ON circle_recipients;
DROP TABLE circle_recipients CASCADE;
DROP TYPE circle_recipient_status;
