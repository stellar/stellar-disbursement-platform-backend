-- This creates the assets table and updates the other tables that depend on it.

-- +migrate Up

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE assets (
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    code VARCHAR(12) NOT NULL,
    issuer VARCHAR(56) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE (code, issuer),
    CONSTRAINT asset_issuer_length_check CHECK (char_length(issuer) = 56)
);
INSERT INTO assets (code, issuer) VALUES ('USDC', 'GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5');

ALTER TABLE disbursements
    ADD COLUMN asset_id VARCHAR(36),
    ADD CONSTRAINT fk_disbursement_asset_id FOREIGN KEY (asset_id) REFERENCES assets (id);
UPDATE disbursements SET asset_id = (SELECT id FROM assets WHERE code = 'USDC' AND issuer = 'GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5');
ALTER TABLE disbursements ALTER COLUMN asset_id SET NOT NULL;

ALTER TABLE payments
    ADD COLUMN asset_id VARCHAR(36),
    ADD CONSTRAINT fk_payment_asset_id FOREIGN KEY (asset_id) REFERENCES assets (id);
UPDATE payments SET asset_id = (SELECT id FROM assets WHERE code = 'USDC' AND issuer = 'GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5');
ALTER TABLE payments ALTER COLUMN asset_id SET NOT NULL;

-- TRIGGER: updated_at
CREATE TRIGGER refresh_asset_updated_at BEFORE UPDATE ON assets FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down

DROP TRIGGER refresh_asset_updated_at ON assets;

ALTER TABLE payments DROP COLUMN asset_id;

ALTER TABLE disbursements DROP COLUMN asset_id;

DROP TABLE assets CASCADE;
