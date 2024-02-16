-- +migrate Up

CREATE TABLE stellar_signatories (
    public_key VARCHAR(64) PRIMARY KEY,
    encrypted_private_key VARCHAR(256),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- TRIGGER: updated_at
CREATE TRIGGER refresh_stellar_signatories_updated_at BEFORE UPDATE ON stellar_signatories FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down

DROP TRIGGER refresh_stellar_signatories_updated_at ON stellar_signatories;

DROP TABLE stellar_signatories;
