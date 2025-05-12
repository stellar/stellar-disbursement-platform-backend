-- This migration extends status_history JSON objects with a status_message key for the receiver_wallets table.
-- It also adds an audit table to the receiver_wallets table to track changes.

-- +migrate Up

-- 1. Extend status_history JSON objects with a status_message key for the receiver_wallets table.
ALTER TABLE receiver_wallets
    ALTER COLUMN status_history DROP DEFAULT;

DROP FUNCTION IF EXISTS create_receiver_wallet_status_history(
    TIMESTAMP WITH TIME ZONE,
    receiver_wallet_status
) CASCADE;

-- +migrate StatementBegin
CREATE FUNCTION create_receiver_wallet_status_history(
    time_stamp TIMESTAMP WITH TIME ZONE,
    rw_status receiver_wallet_status,
    status_message TEXT
) RETURNS JSONB AS $$
    BEGIN
        RETURN json_build_object(
                'timestamp',     time_stamp,
                'status',        rw_status,
                'status_message', status_message
               );
    END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

UPDATE receiver_wallets
SET status_history = (
    SELECT array_agg(jsonb_set(s.elem, '{status_message}', '""'::jsonb))
    FROM (
        SELECT elem
        FROM   unnest(status_history) WITH ORDINALITY AS t(elem, ord)
        ORDER BY ord, elem
    ) s
);

ALTER TABLE receiver_wallets
    ALTER COLUMN status_history
        SET DEFAULT ARRAY[
            create_receiver_wallet_status_history(
                    NOW(),
                    receiver_wallet_status('DRAFT'),
                    ''
            )
        ];


-- 2. Create the audit table for receiver_wallets
SELECT 1 FROM create_audit_table('receiver_wallets');

-- +migrate Down

-- 1. Remove the status_message field from the receiver_wallets table
ALTER TABLE receiver_wallets
    ALTER COLUMN status_history DROP DEFAULT;

DROP FUNCTION IF EXISTS create_receiver_wallet_status_history(
    TIMESTAMP WITH TIME ZONE,
    receiver_wallet_status,
    TEXT
);

-- +migrate StatementBegin
CREATE FUNCTION create_receiver_wallet_status_history(
    time_stamp TIMESTAMP WITH TIME ZONE,
    rw_status receiver_wallet_status
) RETURNS JSONB AS $$
    BEGIN
        RETURN json_build_object(
                'timestamp', time_stamp,
                'status',    rw_status
               );
    END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

ALTER TABLE receiver_wallets
    ALTER COLUMN status_history
        SET DEFAULT ARRAY[
            create_receiver_wallet_status_history(
                    NOW(),
                    receiver_wallet_status('DRAFT')
            )
        ];

-- 2. Drop the audit table for receiver_wallets
SELECT 1 FROM drop_audit_table('receiver_wallets');