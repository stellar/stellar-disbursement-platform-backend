-- +migrate Up
ALTER TABLE circle_transfer_requests
    ADD COLUMN sync_attempts INT DEFAULT 0,
    ADD COLUMN last_sync_attempt_at TIMESTAMPTZ;

-- +migrate Down
ALTER TABLE circle_transfer_requests
    DROP COLUMN sync_attempts,
    DROP COLUMN last_sync_attempt_at;