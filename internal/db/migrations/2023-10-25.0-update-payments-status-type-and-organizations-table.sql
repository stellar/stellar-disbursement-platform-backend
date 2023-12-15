-- +migrate Up
ALTER TYPE payment_status
ADD
    VALUE 'CANCELED';

ALTER TABLE
    public.organizations
ADD
    COLUMN payment_cancellation_period_days INTEGER;

-- +migrate Down
UPDATE
    payments
SET
    status = 'FAILED'::payment_status
WHERE
    status = 'CANCELED'::payment_status;

ALTER TYPE payment_status RENAME TO old_payment_status;

CREATE TYPE payment_status AS ENUM (
    'DRAFT',
    'READY',
    'PENDING',
    'PAUSED',
    'SUCCESS',
    'FAILED'
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

ALTER TABLE payments RENAME COLUMN status TO status_old;
ALTER TABLE payments ADD COLUMN status payment_status DEFAULT payment_status('DRAFT');
UPDATE payments SET status = status_old::text::payment_status;
ALTER TABLE payments DROP COLUMN status_old;
DROP TYPE old_payment_status CASCADE;

ALTER TABLE
    public.organizations DROP COLUMN payment_cancellation_period_days;
