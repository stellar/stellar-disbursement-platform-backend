-- Update the disbursements table.

-- +migrate Up

CREATE TYPE disbursement_status AS ENUM(
    'DRAFT',
    'READY',
    'STARTED',
    'PAUSED',
    'COMPLETED'
);

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION create_disbursement_status_history(time_stamp TIMESTAMP WITH TIME ZONE, disb_status disbursement_status, user_id VARCHAR)
RETURNS jsonb AS $$
	BEGIN
	    RETURN json_build_object(
            'timestamp', time_stamp,
            'status', disb_status,
            'user_id', user_id
        );
	END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

ALTER TABLE public.disbursements
    ALTER COLUMN id SET DEFAULT uuid_generate_v4(),
    ADD COLUMN name VARCHAR(128),
    ADD COLUMN status disbursement_status NOT NULL DEFAULT disbursement_status('DRAFT'),
    ADD COLUMN status_history jsonb[] NOT NULL DEFAULT ARRAY[create_disbursement_status_history(NOW(), disbursement_status('DRAFT'), NULL)],
    ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();
-- TODO: Add column `uploaded_by_user_id` to disbursement table

ALTER TABLE public.disbursements RENAME COLUMN requested_at TO created_at;

-- columns name & id
UPDATE public.disbursements SET name = id;
ALTER TABLE public.disbursements
    ALTER COLUMN created_at SET DEFAULT NOW(),
    ALTER COLUMN name SET NOT NULL,
    ADD CONSTRAINT disbursement_name_unique UNIQUE (name);

-- column status
UPDATE public.disbursements AS d
    SET status = (CASE 
        WHEN EXISTS(SELECT 1 FROM payments WHERE disbursement_id = d.id AND status != 'SUCCESS') THEN disbursement_status('STARTED')
        ELSE disbursement_status('COMPLETED')
    END);

-- column updated_at
CREATE TRIGGER refresh_disbursement_updated_at BEFORE UPDATE ON public.disbursements FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- column status_history
UPDATE public.disbursements SET status_history = ARRAY[create_disbursement_status_history(created_at::TIMESTAMP, disbursement_status('STARTED'), NULL)];
UPDATE public.disbursements SET status_history = array_prepend(create_disbursement_status_history(NOW(), disbursement_status('COMPLETED'), NULL), status_history) WHERE status = disbursement_status('COMPLETED');


-- +migrate Down
DROP TRIGGER refresh_disbursement_updated_at ON public.disbursements;

ALTER TABLE public.disbursements
    DROP CONSTRAINT disbursement_name_unique,
    DROP COLUMN name,
    DROP COLUMN status,
    DROP COLUMN status_history,
    DROP COLUMN updated_at;

ALTER TABLE public.disbursements RENAME COLUMN created_at TO requested_at;

DROP FUNCTION create_disbursement_status_history;

DROP TYPE disbursement_status;