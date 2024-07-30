-- +migrate Up
CREATE TABLE circle_client_config (
    wallet_id VARCHAR(64) NOT NULL,
    encrypted_api_key VARCHAR(256) NOT NULL,
    encrypter_public_key VARCHAR(256) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_single_row_for_circle_client_config()
    RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT COUNT(*) FROM circle_client_config) != 0 THEN
        RAISE EXCEPTION 'circle_client_config must contain exactly one row';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER enforce_single_row_for_circle_client_config_insert_trigger
    BEFORE INSERT ON circle_client_config
    FOR EACH ROW
EXECUTE FUNCTION enforce_single_row_for_circle_client_config();

CREATE TRIGGER enforce_single_row_for_circle_client_config_delete_trigger
    BEFORE DELETE ON circle_client_config
    FOR EACH ROW
EXECUTE FUNCTION enforce_single_row_for_circle_client_config();


-- TRIGGER: updated_at
CREATE TRIGGER refresh_circle_client_config_updated_at BEFORE UPDATE ON circle_client_config FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();


-- +migrate Down

DROP TRIGGER enforce_single_row_for_circle_client_config_delete_trigger ON circle_client_config;

DROP TRIGGER enforce_single_row_for_circle_client_config_insert_trigger ON circle_client_config;

DROP FUNCTION enforce_single_row_for_circle_client_config;

DROP TRIGGER refresh_circle_client_config_updated_at ON circle_client_config;

DROP TABLE circle_client_config;