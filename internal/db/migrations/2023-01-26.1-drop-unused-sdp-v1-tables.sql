-- This migration dumps all django-related stuff that was in the database of the SDP v1.


-- +migrate Up

DROP TABLE IF EXISTS public.on_off_switch CASCADE;
DROP TABLE IF EXISTS public.payments_semaphore CASCADE;
DROP TABLE IF EXISTS public.withdrawal CASCADE;


-- +migrate Down

