-- Update the receiver table.

-- +migrate Up

CREATE TYPE message_status AS ENUM(
    'PENDING',
    'SUCCESS',
    'FAILURE'
);

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION create_message_status_history(time_stamp TIMESTAMP WITH TIME ZONE, m_status message_status, status_message VARCHAR)
RETURNS jsonb AS $$
	BEGIN
        RETURN json_build_object(
            'timestamp', time_stamp,
            'status', m_status,
            'status_message', status_message
        );
	END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

ALTER TABLE public.messages
    ADD COLUMN status message_status NOT NULL DEFAULT message_status('PENDING'),
    ADD COLUMN status_history jsonb[] NOT NULL DEFAULT ARRAY[create_message_status_history(NOW(), message_status('PENDING'), NULL)],
    ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();

-- column updated_at
CREATE TRIGGER refresh_message_updated_at BEFORE UPDATE ON public.messages FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down
DROP TRIGGER refresh_message_updated_at ON public.messages;

ALTER TABLE public.messages
    DROP COLUMN status,
    DROP COLUMN status_history,
    DROP COLUMN updated_at;

DROP FUNCTION create_message_status_history;

DROP TYPE message_status;
