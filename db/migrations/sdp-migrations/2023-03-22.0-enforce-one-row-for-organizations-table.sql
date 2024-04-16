-- Update the organization table to enforce a single row in the whole table.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_single_row_for_organizations()
RETURNS TRIGGER AS $$
BEGIN
  IF (SELECT COUNT(*) FROM organizations) != 0 THEN
    RAISE EXCEPTION 'organizations can must contain exactly one row';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER enforce_single_row_for_organizations_insert_trigger
    BEFORE INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION enforce_single_row_for_organizations();

CREATE TRIGGER enforce_single_row_for_organizations_delete_trigger
    BEFORE DELETE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION enforce_single_row_for_organizations();


-- +migrate Down

DROP TRIGGER enforce_single_row_for_organizations_delete_trigger ON organizations;

DROP TRIGGER enforce_single_row_for_organizations_insert_trigger ON organizations;

DROP FUNCTION enforce_single_row_for_organizations;
