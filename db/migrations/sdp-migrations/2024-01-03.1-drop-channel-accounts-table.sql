-- +migrate Up

DROP TRIGGER refresh_channel_accounts_updated_at ON channel_accounts;

DROP TABLE channel_accounts;


-- +migrate Down

CREATE TABLE channel_accounts (
    public_key VARCHAR(64) PRIMARY KEY,
    private_key VARCHAR(256),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    locked_until_ledger_number INTEGER
);

-- TRIGGER: updated_at
CREATE TRIGGER refresh_channel_accounts_updated_at BEFORE UPDATE ON channel_accounts FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();
