-- +migrate Up
CREATE TABLE auth_user_mfa_codes
(
    device_id         TEXT                                   NOT NULL,
    auth_user_id      VARCHAR(36)                            NOT NULL
        CONSTRAINT fk_mfa_codes_auth_user_id REFERENCES auth_users,
    code              VARCHAR(8),
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    device_expires_at TIMESTAMP WITH TIME ZONE,
    code_expires_at   TIMESTAMP WITH TIME ZONE,
    CONSTRAINT auth_user_mfa_codes_pkey PRIMARY KEY (device_id, auth_user_id),
    UNIQUE (device_id, code)
);

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION auth_user_mfa_codes_before_update()
    RETURNS TRIGGER AS $auth_user_mfa_codes_before_update$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$auth_user_mfa_codes_before_update$ LANGUAGE plpgsql;


CREATE TRIGGER auth_user_mfa_codes_before_update_trigger
    BEFORE UPDATE
    ON auth_user_mfa_codes
    FOR EACH ROW
EXECUTE PROCEDURE auth_user_mfa_codes_before_update();
-- +migrate StatementEnd

-- +migrate Down
DROP TRIGGER auth_user_mfa_codes_before_update_trigger ON auth_user_mfa_codes;
DROP FUNCTION auth_user_mfa_codes_before_update();
DROP TABLE auth_user_mfa_codes;