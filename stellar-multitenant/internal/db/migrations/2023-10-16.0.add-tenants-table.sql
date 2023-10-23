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
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    name text NOT NULL,
    email_sender_type email_sender_type DEFAULT 'DRY_RUN'::email_sender_type,
    sms_sender_type sms_sender_type DEFAULT 'DRY_RUN'::sms_sender_type,
    sep10_signing_public_key text NULL,
    distribution_public_key text NULL,
    enable_mfa boolean DEFAULT true,
    enable_recaptcha boolean DEFAULT true,
    cors_allowed_origins text[] NULL,
    base_url text NULL,
    sdp_ui_base_url text NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_unique_name ON public.tenants (LOWER(name));
CREATE TRIGGER refresh_tenants_updated_at BEFORE UPDATE ON public.tenants FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

COMMENT ON COLUMN tenants.base_url IS 'The SDP backend server''s base URL';
COMMENT ON COLUMN tenants.sdp_ui_base_url IS 'The SDP UI/dashboard Base URL.';

-- +migrate Down

DROP TABLE public.tenants;

DROP TYPE public.email_sender_type;

DROP TYPE public.sms_sender_type;

DROP FUNCTION update_at_refresh;
