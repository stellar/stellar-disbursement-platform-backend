-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION create_message_status_history(time_stamp TIMESTAMP WITH TIME ZONE, m_status message_status, status_message VARCHAR)
RETURNS jsonb AS $$
	BEGIN
        RETURN jsonb_build_object(
            'timestamp', time_stamp,
            'status', m_status,
            'status_message', status_message
        );
	END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

ALTER TABLE public.messages
    ADD COLUMN receiver_wallet_id VARCHAR(36) NULL REFERENCES public.receiver_wallets (id),
    ALTER COLUMN asset_id DROP NOT NULL;

-- Update the receiver_wallet of the messages if we have pre-existing data.
UPDATE 
    public.messages
SET
    receiver_wallet_id = rw.id
FROM (
    SELECT DISTINCT ON (receiver_id) id, receiver_id
    FROM public.receiver_wallets
    ORDER BY receiver_id, id
) AS rw
WHERE
    rw.receiver_id = messages.receiver_id;

-- +migrate Down

ALTER TABLE public.messages
    DROP COLUMN receiver_wallet_id,
    ALTER COLUMN asset_id SET NOT NULL;
