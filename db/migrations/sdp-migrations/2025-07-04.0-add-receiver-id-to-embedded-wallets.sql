-- +migrate Up

ALTER TABLE embedded_wallets
    ADD COLUMN receiver_id VARCHAR(36);

CREATE INDEX idx_embedded_wallets_receiver_id ON embedded_wallets(receiver_id);

ALTER TABLE embedded_wallets
    ADD CONSTRAINT fk_embedded_wallets_receiver_id 
    FOREIGN KEY (receiver_id) REFERENCES receivers(id) ON DELETE CASCADE;

-- +migrate Down

ALTER TABLE embedded_wallets
    DROP CONSTRAINT IF EXISTS fk_embedded_wallets_receiver_id;

DROP INDEX IF EXISTS idx_embedded_wallets_receiver_id;

ALTER TABLE embedded_wallets
    DROP COLUMN receiver_id;