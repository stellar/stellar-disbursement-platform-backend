-- Update the organization table to enforce a single row in the whole table.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_single_row_for_organizations()
RETURNS TRIGGER AS $$
BEGIN
  IF (SELECT COUNT(*) FROM public.organizations) != 0 THEN
    RAISE EXCEPTION 'public.organizations can must contain exactly one row';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER enforce_single_row_for_organizations_insert_trigger
    BEFORE INSERT ON public.organizations
    FOR EACH ROW
    EXECUTE FUNCTION enforce_single_row_for_organizations();

CREATE TRIGGER enforce_single_row_for_organizations_delete_trigger
    BEFORE DELETE ON public.organizations
    FOR EACH ROW
    EXECUTE FUNCTION enforce_single_row_for_organizations();


-- +migrate Down

DROP TRIGGER enforce_single_row_for_organizations_delete_trigger ON public.organizations;

DROP TRIGGER enforce_single_row_for_organizations_insert_trigger ON public.organizations;

DROP FUNCTION enforce_single_row_for_organizations;
