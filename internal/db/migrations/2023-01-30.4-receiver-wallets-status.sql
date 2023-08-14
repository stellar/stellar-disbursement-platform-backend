-- This updates the receiver_wallets by adding a status column, and also removes the status column from the receivers table.
-- The status was moved to the receiver_wallets because a receiver can have multiple wallets and would need to properly register each one of them.

-- +migrate Up

CREATE TYPE receiver_wallet_status AS ENUM(
    'DRAFT',
    'READY',
    'REGISTERED',
    'FLAGGED'
);

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION create_receiver_wallet_status_history(time_stamp TIMESTAMP WITH TIME ZONE, rw_status receiver_wallet_status)
RETURNS jsonb AS $$
	BEGIN
	    RETURN json_build_object(
            'timestamp', time_stamp,
            'status', rw_status
        );
	END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

ALTER TABLE public.receiver_wallets
    ADD COLUMN status receiver_wallet_status NOT NULL DEFAULT receiver_wallet_status('DRAFT'),
    ADD COLUMN status_history jsonb[] NOT NULL DEFAULT ARRAY[create_receiver_wallet_status_history(NOW(), receiver_wallet_status('DRAFT'))];

-- COLUMN: status
UPDATE public.receiver_wallets rwOriginal
    SET status = (
    	CASE WHEN UPPER(r.status) IN ('READY', 'PAID', 'PARTIALLY_CASHED_OUT', 'FULLY_CASHED_OUT') THEN receiver_wallet_status('REGISTERED')
    	ELSE receiver_wallet_status('READY')
    	END
    )
    FROM public.receiver_wallets rw LEFT JOIN public.receivers r ON rw.receiver_id = r.id
    WHERE rwOriginal.id = rw.id;

-- COLUMN: status_history
UPDATE public.receiver_wallets rwOriginal
    SET status_history = (
        CASE WHEN rwOriginal.status = receiver_wallet_status('REGISTERED') THEN ARRAY[create_receiver_wallet_status_history(NOW(), receiver_wallet_status('REGISTERED'))]
        ELSE ARRAY[create_receiver_wallet_status_history(NOW(), receiver_wallet_status('READY'))]
        END
    )
    FROM public.receiver_wallets rw LEFT JOIN public.receivers r ON rw.receiver_id = r.id
    WHERE rwOriginal.id = rw.id;

-- TABLE: receiver
ALTER TABLE public.receivers DROP COLUMN status;


-- +migrate Down

-- TABLE: receiver
ALTER TABLE public.receivers ADD COLUMN status VARCHAR(32);

ALTER TABLE public.receiver_wallets
    DROP COLUMN status,
    DROP COLUMN status_history;

DROP FUNCTION create_receiver_wallet_status_history;

DROP TYPE receiver_wallet_status;
