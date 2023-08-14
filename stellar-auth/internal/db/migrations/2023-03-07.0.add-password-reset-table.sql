-- +migrate Up

CREATE TABLE
    public.auth_user_password_reset (
        token text NOT NULL UNIQUE,
        auth_user_id VARCHAR(36) NOT NULL,
        is_valid boolean NOT NULL DEFAULT true,
        created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
        CONSTRAINT fk_password_reset_auth_user_id
            FOREIGN KEY (auth_user_id)
                REFERENCES auth_users(id)
    );

CREATE UNIQUE INDEX unique_user_valid_token ON auth_user_password_reset(auth_user_id, is_valid) WHERE (is_valid IS TRUE);

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION auth_user_password_reset_before_insert()
RETURNS TRIGGER AS $auth_user_password_reset_before_insert$
BEGIN
    UPDATE
        auth_user_password_reset
    SET
        is_valid = false
    WHERE
        auth_user_id = NEW.auth_user_id;

    RETURN NEW;
END;
$auth_user_password_reset_before_insert$ LANGUAGE plpgsql;


CREATE TRIGGER auth_user_password_reset_before_insert_trigger
BEFORE INSERT
ON auth_user_password_reset
FOR EACH ROW
EXECUTE PROCEDURE auth_user_password_reset_before_insert();
-- +migrate StatementEnd

-- +migrate Down

DROP TRIGGER auth_user_password_reset_before_insert_trigger ON auth_user_password_reset;
DROP FUNCTION IF EXISTS auth_user_password_reset_before_insert;
DROP INDEX IF EXISTS unique_user_valid_token;
DROP TABLE public.auth_user_password_reset;
