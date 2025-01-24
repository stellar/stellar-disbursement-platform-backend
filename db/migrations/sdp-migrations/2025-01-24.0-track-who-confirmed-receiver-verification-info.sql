-- Purpose: Add columns to track who confirmed receiver verification info

-- +migrate Up
CREATE TYPE confirmed_by_type AS ENUM ('RECEIVER', 'USER');

ALTER TABLE receiver_verifications
    ADD COLUMN confirmed_by_type confirmed_by_type NOT NULL DEFAULT 'RECEIVER',
    ADD COLUMN confirmed_by_id VARCHAR(36);

UPDATE receiver_verifications SET confirmed_by_id = receiver_id WHERE confirmed_at IS NOT NULL;

-- Add constraint where confirmed_by_id cannot be empty if confirmed_at is populated
ALTER TABLE receiver_verifications
    ADD CONSTRAINT confirmed_by_id_not_empty_when_completed CHECK (confirmed_by_id IS NOT NULL OR confirmed_at IS NULL);


-- +migrate Down
ALTER TABLE receiver_verifications
    DROP CONSTRAINT confirmed_by_id_not_empty_when_completed,
    DROP COLUMN confirmed_by,
    DROP COLUMN confirmed_by_id;

DROP TYPE confirmed_by_type;
