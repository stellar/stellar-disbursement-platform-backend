-- +migrate Up
CREATE TABLE receiver_registration_attempts (
    id SERIAL PRIMARY KEY,
    phone_number VARCHAR(32),
    email VARCHAR(254),
    attempt_ts TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    client_domain TEXT NOT NULL,
    transaction_id TEXT NOT NULL,
    wallet_address TEXT NOT NULL,
    wallet_memo TEXT
);
-- Add indexes for faster lookups
CREATE INDEX idx_receiver_reg_attempts_phone ON receiver_registration_attempts (phone_number);

CREATE INDEX idx_receiver_reg_attempts_email ON receiver_registration_attempts (email);

CREATE INDEX idx_receiver_registration_attempts_attempt_ts ON receiver_registration_attempts (attempt_ts);

-- +migrate Down
DROP TABLE IF EXISTS receiver_registration_attempts;