-- +migrate Up

CREATE INDEX idx_embedded_wallets_receiver_contact_type_created_at 
ON embedded_wallets (receiver_contact, contact_type, created_at DESC);

-- +migrate Down

DROP INDEX IF EXISTS idx_embedded_wallets_receiver_contact_type_created_at;