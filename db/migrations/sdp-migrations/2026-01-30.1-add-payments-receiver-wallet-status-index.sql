-- +migrate Up

CREATE INDEX payments_receiver_wallet_type_status_updated_at_idx
    ON payments (receiver_wallet_id, type, status, updated_at DESC);

-- +migrate Down

DROP INDEX IF EXISTS payments_receiver_wallet_type_status_updated_at_idx;
