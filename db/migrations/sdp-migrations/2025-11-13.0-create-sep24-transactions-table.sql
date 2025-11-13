-- +migrate Up

CREATE TABLE sep24_transactions (
    id VARCHAR(36) PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sep24_transactions_created_at ON sep24_transactions (created_at);

-- +migrate Down

DROP INDEX IF EXISTS idx_sep24_transactions_created_at;
DROP TABLE sep24_transactions;

