-- +migrate Up

CREATE TABLE vault (
    public_key VARCHAR(64) PRIMARY KEY,
    encrypted_private_key VARCHAR(256),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- TRIGGER: updated_at
CREATE TRIGGER refresh_vault_updated_at BEFORE UPDATE ON vault FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down

DROP TRIGGER refresh_vault_updated_at ON vault;

DROP TABLE vault;
