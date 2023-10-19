-- This migration dumps all django-related stuff that was in the database of the SDP v1.


-- +migrate Up

DROP TABLE IF EXISTS public.on_off_switch CASCADE;
DROP TABLE IF EXISTS public.payments_semaphore CASCADE;
ALTER TABLE public.withdrawal RENAME COLUMN account_id TO receiver_id;

-- +migrate StatementBegin
-- Delete withdrawal table if it is empty
DO $$
BEGIN 
    IF (SELECT COUNT(*) FROM public.withdrawal) = 0 THEN 
        EXECUTE 'DROP TABLE public.withdrawal'; 
    END IF; 
END $$;
-- +migrate StatementEnd


-- +migrate Down
