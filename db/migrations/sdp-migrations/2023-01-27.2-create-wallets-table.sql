-- This creates the wallets table and updates the other tables that depend on it.

-- +migrate Up

CREATE TABLE wallets (
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    name VARCHAR(30) NOT NULL,
    homepage VARCHAR(255) NOT NULL,
    deep_link_schema VARCHAR(30) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE (name),
    UNIQUE (homepage),
    UNIQUE (deep_link_schema)
);
-- TODO: keep in mind that the deep link `vibrantapp://` is not confirmed yet and is subject to change.
INSERT INTO wallets (name, homepage, deep_link_schema) VALUES ('Vibrant Assist', 'https://vibrantapp.com', 'https://vibrantapp.com/sdp-dev');

ALTER TABLE disbursements
    ADD COLUMN wallet_id VARCHAR(36),
    ADD CONSTRAINT fk_disbursement_wallet_id FOREIGN KEY (wallet_id) REFERENCES wallets (id);
UPDATE disbursements SET wallet_id = (SELECT id FROM wallets WHERE name = 'Vibrant Assist');
ALTER TABLE disbursements ALTER COLUMN wallet_id SET NOT NULL;

CREATE TRIGGER refresh_wallet_updated_at BEFORE UPDATE ON wallets FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down
DROP TRIGGER refresh_wallet_updated_at ON wallets;

ALTER TABLE disbursements DROP COLUMN wallet_id;

DROP TABLE wallets CASCADE;
