-- Update the payments table.

-- +migrate Up

CREATE TYPE payment_status AS ENUM(
    'DRAFT',
    'READY',
    'PENDING',
    'PAUSED',
    'SUCCESS',
    'FAILURE'
);

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION create_payment_status_history(time_stamp TIMESTAMP WITH TIME ZONE, pay_status payment_status, status_message VARCHAR)
RETURNS jsonb AS $$
	BEGIN
        RETURN json_build_object(
            'timestamp', time_stamp,
            'status', pay_status,
            'status_message', status_message
        );
	END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

ALTER TABLE public.payments RENAME COLUMN requested_at TO created_at;
ALTER TABLE public.payments RENAME COLUMN account_id TO receiver_id;
ALTER TABLE public.payments RENAME COLUMN status TO old_status;
ALTER TABLE public.payments
    ALTER COLUMN id SET DEFAULT uuid_generate_v4(),
    ALTER COLUMN amount TYPE numeric(19, 7),
    ALTER COLUMN status_message TYPE VARCHAR(256),
    ALTER COLUMN created_at SET DEFAULT NOW(),
    DROP COLUMN custodial_payment_id,
    DROP COLUMN idempotency_key,
    DROP COLUMN withdrawal_amount,
    DROP COLUMN withdrawal_status,
    ADD COLUMN stellar_operation_id VARCHAR(32),
    ADD COLUMN blockchain_sender_id VARCHAR(69),
    ADD COLUMN status payment_status NOT NULL DEFAULT payment_status('DRAFT'),
    ADD COLUMN status_history jsonb[] NOT NULL DEFAULT ARRAY[create_payment_status_history(NOW(), payment_status('DRAFT'), NULL)],
    ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();

-- column status
UPDATE public.payments AS d
    SET status = (CASE 
        WHEN old_status='REQUESTED' THEN payment_status('READY')
        WHEN old_status='PENDING' THEN payment_status('PENDING')
        WHEN old_status='PENDING_FUNDS' THEN payment_status('FAILURE')
        WHEN old_status='SUCCESS' THEN payment_status('SUCCESS')
        ELSE payment_status('FAILURE')
    END);

-- column status_history
UPDATE public.payments SET status_history = ARRAY[create_payment_status_history(created_at::TIMESTAMP, payment_status('READY'), NULL)];
UPDATE public.payments SET status_history = array_prepend(create_payment_status_history(started_at::TIMESTAMP, payment_status('PENDING'), NULL), status_history) WHERE started_at IS NOT NULL;
UPDATE public.payments SET status_history = array_prepend(create_payment_status_history(NOW(), payment_status('FAILURE'), status_message::VARCHAR), status_history) WHERE old_status='PENDING_FUNDS';
UPDATE public.payments SET status_history = array_prepend(create_payment_status_history(completed_at::TIMESTAMP, payment_status('SUCCESS'), NULL), status_history) WHERE old_status='SUCCESS';
UPDATE public.payments SET status_history = array_prepend(create_payment_status_history(completed_at::TIMESTAMP, payment_status('FAILURE'), status_message::VARCHAR), status_history) WHERE old_status='FAILURE';

-- column updated_at
CREATE TRIGGER refresh_payment_updated_at BEFORE UPDATE ON public.payments FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down
DROP TRIGGER refresh_payment_updated_at ON public.payments;

ALTER TABLE public.payments
    ADD COLUMN custodial_payment_id VARCHAR(36),
    ADD COLUMN idempotency_key VARCHAR(64),
    ADD COLUMN withdrawal_amount NUMERIC(7,2) NOT NULL DEFAULT 0,
    ADD COLUMN withdrawal_status VARCHAR(32),
    DROP COLUMN updated_at,
    DROP COLUMN status,
    DROP COLUMN status_history,
    DROP COLUMN stellar_operation_id,
    DROP COLUMN blockchain_sender_id;

ALTER TABLE public.payments RENAME COLUMN old_status TO status;
ALTER TABLE public.payments RENAME COLUMN created_at TO requested_at;
ALTER TABLE public.payments RENAME COLUMN receiver_id TO account_id;

DROP FUNCTION create_payment_status_history;

DROP TYPE payment_status;