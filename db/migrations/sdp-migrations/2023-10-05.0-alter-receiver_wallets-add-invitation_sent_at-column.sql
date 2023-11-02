-- +migrate Up
ALTER TABLE
    receiver_wallets
ADD
    COLUMN invitation_sent_at timestamp with time zone;

UPDATE
    receiver_wallets
SET
    invitation_sent_at = NOW()
WHERE
    status = 'REGISTERED';
    
-- +migrate Down
ALTER TABLE
    receiver_wallets DROP COLUMN invitation_sent_at;
    