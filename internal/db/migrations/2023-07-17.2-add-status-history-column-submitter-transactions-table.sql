-- This migration updates the submitter_transactions table by adding the status_history column, for increased debuggability.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION create_submitter_transactions_status_history(time_stamp TIMESTAMP WITH TIME ZONE, tss_status transaction_status, status_message VARCHAR, stellar_transaction_hash TEXT, xdr_sent TEXT, xdr_received TEXT)
RETURNS jsonb AS $$
	BEGIN
	    RETURN json_build_object(
            'timestamp', time_stamp,
            'status', tss_status,
            'status_message', status_message,
            'stellar_transaction_hash', stellar_transaction_hash,
            'xdr_sent', xdr_sent,
            'xdr_received', xdr_received
        );
	END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

ALTER TABLE public.submitter_transactions
    ADD COLUMN status_history jsonb[] NULL DEFAULT ARRAY[create_submitter_transactions_status_history(NOW(), 'PENDING', NULL, NULL, NULL, NULL)];


-- +migrate Down

ALTER TABLE public.submitter_transactions
    DROP COLUMN status_history;

DROP FUNCTION IF EXISTS create_submitter_transactions_status_history;
