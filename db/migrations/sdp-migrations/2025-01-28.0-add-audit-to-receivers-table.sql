-- Add auditing to receiver_verifications

-- +migrate Up
SELECT 1 FROM create_audit_table('receivers');

-- +migrate Down
SELECT 1 FROM drop_audit_table('receivers');