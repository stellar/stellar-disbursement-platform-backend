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

-- +migrate Down

ALTER TABLE public.messages
    DROP COLUMN receiver_wallet_id,
    ALTER COLUMN asset_id SET NOT NULL;
