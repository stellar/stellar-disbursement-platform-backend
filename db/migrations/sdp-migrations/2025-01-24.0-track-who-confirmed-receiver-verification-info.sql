-- Purpose: Add columns to track who confirmed receiver verification info

-- +migrate Up
CREATE TYPE confirmed_by_type AS ENUM ('RECEIVER', 'USER');

ALTER TABLE receiver_verifications
    ADD COLUMN confirmed_by_type confirmed_by_type,
    ADD COLUMN confirmed_by_id VARCHAR(36);

UPDATE
    receiver_verifications
SET
    confirmed_by_id = receiver_id,
    confirmed_by_type = 'RECEIVER'
WHERE
    confirmed_at IS NOT NULL;


-- +migrate Down
ALTER TABLE receiver_verifications
    DROP COLUMN confirmed_by_type,
    DROP COLUMN confirmed_by_id;

DROP TYPE confirmed_by_type;
