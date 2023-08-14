-- Add function used to refresh the updated_at column automatically.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION update_at_refresh()   
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;   
END;
$$ language 'plpgsql';
-- +migrate StatementEnd


-- +migrate Down

DROP FUNCTION update_at_refresh;
