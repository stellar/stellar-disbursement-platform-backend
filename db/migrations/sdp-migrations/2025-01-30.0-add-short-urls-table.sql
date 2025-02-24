-- Add auditing to receiver_verifications

-- +migrate Up
CREATE TABLE short_urls (
    id VARCHAR(10) PRIMARY KEY,
    original_url TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX short_urls_original_url_idx ON short_urls (original_url);

ALTER TABLE organizations
    ADD COLUMN is_link_shortener_enabled boolean NOT NULL DEFAULT false;


-- +migrate Down
DROP TABLE short_urls;

ALTER TABLE organizations
    DROP COLUMN is_link_shortener_enabled;