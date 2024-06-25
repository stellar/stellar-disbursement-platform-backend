-- +migrate Up
CREATE TYPE circle_transfer_status AS ENUM ('pending', 'complete', 'failed');

CREATE TABLE circle_transfer_requests (
    idempotency_key VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    payment_id VARCHAR(36) NOT NULL,
    circle_transfer_id VARCHAR(36),
    status circle_transfer_status,
    response_body JSONB,
    source_wallet_id VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_payment_id ON circle_transfer_requests (payment_id) WHERE (status IS DISTINCT FROM 'failed');

-- TRIGGER: updated_at
CREATE TRIGGER refresh_circle_transfer_requests_updated_at BEFORE UPDATE ON circle_transfer_requests FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- +migrate Down
DROP TRIGGER refresh_circle_transfer_requests_updated_at ON circle_transfer_requests;

DROP TABLE circle_transfer_requests;

DROP TYPE circle_transfer_status;
