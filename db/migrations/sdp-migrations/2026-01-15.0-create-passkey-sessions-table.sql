-- +migrate Up

CREATE TABLE IF NOT EXISTS passkey_sessions (
    challenge TEXT PRIMARY KEY,
    session_type TEXT NOT NULL,
    session_data JSONB NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS passkey_sessions_expires_at_idx ON passkey_sessions (expires_at);

-- +migrate Down

DROP INDEX IF EXISTS passkey_sessions_expires_at_idx;
DROP TABLE IF EXISTS passkey_sessions;
