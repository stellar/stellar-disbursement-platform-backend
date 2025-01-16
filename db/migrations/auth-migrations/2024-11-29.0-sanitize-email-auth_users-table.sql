-- +migrate Up
UPDATE auth_users
SET
    email = LOWER(TRIM(email));

-- +migrate Down
-- No down migration needed as email sanitization cannot be reversed