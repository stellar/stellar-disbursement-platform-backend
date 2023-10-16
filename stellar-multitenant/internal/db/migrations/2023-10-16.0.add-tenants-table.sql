-- +migrate Up

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION update_at_refresh()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';
-- +migrate StatementEnd

CREATE TYPE public.email_sender_type AS ENUM ('AWS_EMAIL', 'DRY_RUN');
CREATE TYPE public.sms_sender_type AS ENUM ('TWILIO_SMS', 'AWS_SMS', 'DRY_RUN');

CREATE TABLE public.tenants
(
    id                       VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    name                     text              not null,
    email_sender_type        email_sender_type not null,
    sms_sender_type          sms_sender_type   not null,
    sep10_signing_public_key text              not null,
    distribution_public_key  text              not null,
    enable_mfa               boolean           not null,
    enable_recaptcha         boolean           not null,
    cors_allowed_origins     text              not null,
    base_url                 text              not null,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        unique(name)
);

CREATE TRIGGER refresh_tenants_updated_at BEFORE UPDATE ON public.tenants FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- +migrate Down

DROP TABLE public.tenants;

DROP TYPE public.email_sender_type;

DROP TYPE public.sms_sender_type;

DROP FUNCTION update_at_refresh;
