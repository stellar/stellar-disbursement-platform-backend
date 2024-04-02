-- +migrate Up
ALTER TABLE
    organizations
ADD
    COLUMN privacy_policy_link TEXT NULL;

-- +migrate Down
ALTER TABLE
    organizations DROP COLUMN privacy_policy_link;