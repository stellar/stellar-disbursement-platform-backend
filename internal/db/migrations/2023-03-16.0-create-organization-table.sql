-- This creates the organizations table.

-- +migrate Up

-- Table: organizations
CREATE TABLE public.organizations (
    id VARCHAR(36) PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(64) NOT NULL,
    stellar_main_address VARCHAR(56) NOT NULL,
    timezone_utc_offset VARCHAR(6) NOT NULL DEFAULT '+00:00',
    are_payments_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    sms_registration_message_template VARCHAR(255) NOT NULL DEFAULT 'You have a payment waiting for you from the {{.OrganizationName}}. Click {{.RegistrationLink}} to register.',

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    UNIQUE (name),
    CONSTRAINT organization_name_not_empty_check CHECK (char_length(name) > 1),
    CONSTRAINT organization_timezone_size_check CHECK (char_length(timezone_utc_offset) = 6),
    CONSTRAINT organization_sms_registration_message_template_contains_tags_check CHECK (sms_registration_message_template LIKE '%{{.OrganizationName}}%' AND sms_registration_message_template LIKE '%{{.RegistrationLink}}%')
);

INSERT INTO public.organizations (name, stellar_main_address) VALUES ('MyCustomAid', 'GDA34JZ26FZY64XCSY46CUNSHLX762LHJXQHWWHGL5HSFRWSGBVHUFNI');

CREATE TRIGGER refresh_organization_updated_at BEFORE UPDATE ON public.organizations FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down

-- Table: organizations
DROP TRIGGER refresh_organization_updated_at ON public.organizations;

DROP TABLE public.organizations CASCADE;
