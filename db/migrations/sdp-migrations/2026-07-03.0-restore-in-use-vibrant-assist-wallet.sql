-- Restore Vibrant Assist wallet where 2026-01-16.0 soft-deleted it despite >= 1 associated receiver_wallets.

-- +migrate Up
UPDATE wallets
SET
    enabled = TRUE,
    deleted_at = NULL
WHERE name = 'Vibrant Assist'
    AND deleted_at IS NOT NULL
    AND EXISTS (
        SELECT 1
        FROM receiver_wallets
        WHERE wallet_id = wallets.id
    );

-- +migrate Down
