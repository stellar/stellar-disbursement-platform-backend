-- Rename blockchain_sender_id column to sender_address for clarity.
-- This column stores the distribution account address used to send each payment.

-- +migrate Up
ALTER TABLE payments RENAME COLUMN blockchain_sender_id TO sender_address;

-- +migrate Down
ALTER TABLE payments RENAME COLUMN sender_address TO blockchain_sender_id;
