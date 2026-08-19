-- Add a 'CANCELED' status to the disbursement_status enum. This lets a DRAFT/READY
-- disbursement (never started, so no on-chain submissions in flight) be canceled in bulk,
-- releasing its reserved capacity, instead of requiring each payment to be canceled one by one.
-- STARTED disbursements are out of scope for cancellation - PAUSE is the correct action there.

-- +migrate Up
ALTER TYPE disbursement_status
ADD
    VALUE 'CANCELED';

-- +migrate Down
-- No disbursement should ever have reached CANCELED before this migration existed, but guard
-- anyway: without a mapping back to a pre-existing status, downgrading a CANCELED disbursement
-- would violate the enum being recreated below. Map it to the closest terminal equivalent. This
-- only fixes up the live `disbursements` row - disbursements_audit is append-only and
-- intentionally NOT rewritten here (see below).
UPDATE
    disbursements
SET
    status = 'COMPLETED'::disbursement_status
WHERE
    status = 'CANCELED'::disbursement_status;

-- Column defaults are bound to the enum's current OID, so they block the type swap below - drop
-- them first, reinstate after.
ALTER TABLE disbursements ALTER COLUMN status DROP DEFAULT;
ALTER TABLE disbursements ALTER COLUMN status_history DROP DEFAULT;

ALTER TYPE disbursement_status RENAME TO old_disbursement_status;

-- The status-history helper function's parameter is bound to the old enum's OID too; drop it so
-- old_disbursement_status has no remaining dependents once every column below is retyped.
DROP FUNCTION create_disbursement_status_history(TIMESTAMP WITH TIME ZONE, old_disbursement_status, VARCHAR);

CREATE TYPE disbursement_status AS ENUM (
    'DRAFT',
    'READY',
    'STARTED',
    'PAUSED',
    'COMPLETED'
);

-- +migrate StatementBegin
CREATE FUNCTION create_disbursement_status_history(time_stamp TIMESTAMP WITH TIME ZONE, disb_status disbursement_status, user_id VARCHAR)
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

ALTER TABLE disbursements
    ALTER COLUMN status TYPE disbursement_status USING status::text::disbursement_status;

ALTER TABLE disbursements
    ALTER COLUMN status SET DEFAULT disbursement_status('DRAFT');

ALTER TABLE disbursements
    ALTER COLUMN status_history SET DEFAULT ARRAY[create_disbursement_status_history(NOW(), disbursement_status('DRAFT'), NULL)];

-- disbursements_audit mirrors the column's type (create_audit_table snapshots it at creation
-- time, with no default), so it must be retyped too before the old enum can be dropped. It's
-- append-only and deliberately not rewritten above, so this correctly fails instead if any
-- historical row ever recorded CANCELED - shrinking an enum that audit history already used is
-- not something that can be done without rewriting that history.
ALTER TABLE disbursements_audit
    ALTER COLUMN status TYPE disbursement_status USING status::text::disbursement_status;

DROP TYPE old_disbursement_status;
