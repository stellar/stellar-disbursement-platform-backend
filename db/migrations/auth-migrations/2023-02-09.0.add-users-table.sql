-- +migrate Up

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE
    auth_users (
        id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
        username text NOT NULL,
        encrypted_password text NOT NULL,
        email text NOT NULL,
        is_owner boolean NOT NULL DEFAULT false,
        created_at TIMESTAMP
        WITH
            TIME ZONE NOT NULL DEFAULT NOW(),
            UNIQUE (username),
            UNIQUE (email)
    );

-- +migrate Down

DROP TABLE auth_users;
