-- Adds new assets, countries and wallets to the database

-- +migrate Up

-- Add USA, BRA and COL to the countries table
INSERT INTO
    public.countries (code, name)
VALUES
    ('BRA', 'Brazil'),
    ('USA', 'United States of America'),
    ('COL', 'Colombia');

-- +migrate Down

-- Remove USA, BRA and COL from the countries table
DELETE FROM
    public.countries
WHERE
    code IN ('BRA', 'USA', 'COL');
