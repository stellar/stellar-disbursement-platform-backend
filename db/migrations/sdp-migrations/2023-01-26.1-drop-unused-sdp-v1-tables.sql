-- This migration dumps all django-related stuff that was in the database of the SDP v1.


-- +migrate Up

DROP TABLE IF EXISTS on_off_switch CASCADE;
DROP TABLE IF EXISTS payments_semaphore CASCADE;
ALTER TABLE withdrawal RENAME COLUMN account_id TO receiver_id;

-- +migrate StatementBegin
-- Delete withdrawal table if it is empty
DO $$
BEGIN 
    IF (SELECT COUNT(*) FROM withdrawal) = 0 THEN 
        EXECUTE 'DROP TABLE withdrawal'; 
    END IF; 
END $$;
-- +migrate StatementEnd


-- +migrate Down
