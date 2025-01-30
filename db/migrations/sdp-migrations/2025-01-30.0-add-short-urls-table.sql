-- Add auditing to receiver_verifications

-- +migrate Up
CREATE TABLE short_urls (
    id VARCHAR(10) PRIMARY KEY,
    original_url TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    hits BIGINT DEFAULT 0
);

CREATE INDEX short_urls_original_url_idx ON short_urls (original_url);

-- +migrate Down
DROP TABLE short_urls;