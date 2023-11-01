-- This creates the receiver_wallets table and updates the other tables that depend on it.

-- +migrate Up

-- Table: receiver_wallets
CREATE TABLE receiver_wallets (
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    receiver_id VARCHAR(36) NOT NULL REFERENCES receivers (id),
    wallet_id VARCHAR(36) REFERENCES wallets (id),
    stellar_address VARCHAR(56),
    stellar_memo VARCHAR(56),
    stellar_memo_type VARCHAR(56),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (receiver_id, wallet_id)
);
INSERT
    INTO receiver_wallets (receiver_id, stellar_address) 
    (SELECT id, public_key FROM receivers);
UPDATE receiver_wallets SET wallet_id = (SELECT id FROM wallets WHERE name = 'Vibrant Assist');
ALTER TABLE receiver_wallets ALTER COLUMN wallet_id SET NOT NULL;

-- Table: receivers
ALTER TABLE receivers DROP COLUMN public_key;

CREATE TRIGGER refresh_receiver_wallet_updated_at BEFORE UPDATE ON receiver_wallets FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down
DROP TRIGGER refresh_receiver_wallet_updated_at ON receiver_wallets;

-- Table: receivers
ALTER TABLE receivers ADD COLUMN public_key VARCHAR(128);
UPDATE receivers SET public_key = (SELECT stellar_address FROM receiver_wallets WHERE receiver_id = receivers.id);

-- Table: receiver_wallets
DROP TABLE receiver_wallets CASCADE;
