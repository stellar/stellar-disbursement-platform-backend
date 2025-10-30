-- Enable short link feature by default

-- +migrate Up
ALTER TABLE organizations
    ALTER COLUMN is_link_shortener_enabled SET DEFAULT true;

-- +migrate Down
ALTER TABLE organizations
    ALTER COLUMN is_link_shortener_enabled SET DEFAULT false;
