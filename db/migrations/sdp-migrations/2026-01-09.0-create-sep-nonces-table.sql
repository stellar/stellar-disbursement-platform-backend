-- +migrate Up

CREATE TABLE IF NOT EXISTS sep_nonces (
    nonce TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS sep_nonces_expires_at_idx ON sep_nonces (expires_at);

-- +migrate Down

DROP INDEX IF EXISTS sep_nonces_expires_at_idx;
DROP TABLE IF EXISTS sep_nonces;
