-- This creates the countries table and updates the other tables that depend on it.

-- +migrate Up

CREATE TABLE countries (
    code VARCHAR(3) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,
    UNIQUE (name),
    CONSTRAINT country_code_length_check CHECK (char_length(code) = 3)
);
INSERT INTO countries (code, name) VALUES ('UKR', 'Ukraine');

ALTER TABLE disbursements
    ADD COLUMN country_code VARCHAR(3),
    ADD CONSTRAINT fk_disbursement_country_code FOREIGN KEY (country_code) REFERENCES countries (code);
UPDATE disbursements SET country_code = 'UKR';
ALTER TABLE disbursements ALTER COLUMN country_code SET NOT NULL;

CREATE TRIGGER refresh_country_updated_at BEFORE UPDATE ON countries FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- +migrate Down
ALTER TABLE disbursements DROP COLUMN country_code CASCADE;

DROP TABLE countries CASCADE;
