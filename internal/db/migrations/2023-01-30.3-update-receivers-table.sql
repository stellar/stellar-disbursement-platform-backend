-- Update the receiver table.

-- +migrate Up

ALTER TABLE public.receivers RENAME COLUMN registered_at TO created_at;
ALTER TABLE public.receivers
    ALTER COLUMN id SET DEFAULT uuid_generate_v4(),
    ALTER COLUMN created_at SET DEFAULT NOW(),
    DROP COLUMN link_last_sent_at,
    DROP COLUMN email_registered_at,
    DROP COLUMN public_key_registered_at,
    DROP COLUMN hashed_extra_info,
    DROP COLUMN hashed_phone_number,
    ADD COLUMN updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW();

-- column updated_at
CREATE TRIGGER refresh_receiver_updated_at BEFORE UPDATE ON public.receivers FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down
DROP TRIGGER refresh_receiver_updated_at ON public.receivers;

ALTER TABLE public.receivers
    ADD COLUMN link_last_sent_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN email_registered_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN public_key_registered_at TIMESTAMP WITH TIME ZONE,
    ADD COLUMN hashed_extra_info VARCHAR(64),
    ADD COLUMN hashed_phone_number VARCHAR(64),
    DROP COLUMN updated_at;

ALTER TABLE public.receivers RENAME COLUMN created_at TO registered_at;
