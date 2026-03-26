-- Soft delete the legacy Vibrant Assist wallet so it is no longer returned by /wallets.

-- +migrate Up
UPDATE wallets
SET
    enabled = FALSE,
    deleted_at = COALESCE(deleted_at, NOW())
WHERE name = 'Vibrant Assist';

-- +migrate Down
UPDATE wallets
SET
    enabled = TRUE,
    deleted_at = NULL
WHERE name = 'Vibrant Assist';
