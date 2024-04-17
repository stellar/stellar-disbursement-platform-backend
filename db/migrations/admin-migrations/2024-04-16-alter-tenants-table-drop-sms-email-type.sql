-- +migrate Up
ALTER TABLE public.tenants
    DROP COLUMN email_sender_type,
    DROP COLUMN sms_sender_type;

DROP TYPE public.email_sender_type;
DROP TYPE public.sms_sender_type;

-- +migrate Down

CREATE TYPE public.email_sender_type AS ENUM ('AWS_EMAIL', 'DRY_RUN');
CREATE TYPE public.sms_sender_type AS ENUM ('TWILIO_SMS', 'AWS_SMS', 'DRY_RUN');

ALTER TABLE public.tenants
    ADD COLUMN email_sender_type email_sender_type DEFAULT 'DRY_RUN'::email_sender_type,
    ADD COLUMN sms_sender_type sms_sender_type DEFAULT 'DRY_RUN'::sms_sender_type;