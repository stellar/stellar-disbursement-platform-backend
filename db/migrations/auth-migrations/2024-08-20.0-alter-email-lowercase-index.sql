-- +migrate Up

CREATE UNIQUE INDEX lower_case_email ON auth_users (LOWER(email));

-- +migrate Down

DROP INDEX IF EXISTS lower_case_email;