-- +migrate Up
ALTER TABLE
    public.receiver_wallets
ADD
    COLUMN invitation_sent_at timestamp with time zone;

UPDATE
    public.receiver_wallets
SET
    invitation_sent_at = NOW()
WHERE
    status = 'REGISTERED';
    
-- +migrate Down
ALTER TABLE
    public.receiver_wallets DROP COLUMN invitation_sent_at;
    