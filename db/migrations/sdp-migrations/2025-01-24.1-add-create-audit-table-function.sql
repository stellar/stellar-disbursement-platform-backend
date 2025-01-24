-- Add a new function `create_audit_table` to create an audit table for a given table name

-- +migrate Up

-- `create_audit_table` adds auditing to a table by creating an audit table and a trigger function.
-- 1. The audit table named `table_name_audit` is created with the same columns as the original table plus two additional columns:
-- - operation: text, NOT NULL, the operation that triggered the audit (INSERT, UPDATE, DELETE)
-- - changed_at: timestamptz, NOT NULL, the timestamp of the operation
-- 2. The trigger function `table_name_audit_fn` is created to handle the INSERT, UPDATE, DELETE operations on the original table.
-- 3. The trigger `table_name_audit_trigger` is created on the original table to call the trigger function on each operation.

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION create_audit_table(p_table text)
    RETURNS void
    LANGUAGE plpgsql
AS $$
DECLARE
    v_schema_name       text;
    v_table_name        text;
    v_audit_name      text;

    -- For CREATE TABLE:
    v_col_def_list      text := '';  -- e.g. id bigint, name text, ...

    -- For INSERT statements:
    v_col_name_list     text := '';  -- e.g. id, name, ...
    v_select_list_new   text := '';  -- e.g. NEW.id, NEW.name, ...
    v_select_list_old   text := '';  -- e.g. OLD.id, OLD.name, ...

    rec record;
BEGIN
    -------------------------------------------------------------------
    -- 1) Separate out schema vs. table (default to public if no dot)
    -------------------------------------------------------------------
    IF p_table LIKE '%.%' THEN
        v_schema_name := split_part(p_table, '.', 1);
        v_table_name  := split_part(p_table, '.', 2);
    ELSE
        v_schema_name := "current_schema"();
        v_table_name  := p_table;
    END IF;

    -- Derive audit table name (same schema, table + "_audit")
    v_audit_name := v_table_name || '_audit';

    -------------------------------------------------------------------
    -- 2) Gather columns from catalogs using format_type()
    -------------------------------------------------------------------
    FOR rec IN
        SELECT a.attname AS column_name,
               pg_catalog.format_type(a.atttypid, a.atttypmod) AS full_data_type
        FROM pg_catalog.pg_attribute a
                 JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
                 JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = v_table_name
          AND n.nspname = v_schema_name
          AND a.attnum > 0 -- only shows user defined columns.
          AND NOT a.attisdropped
        ORDER BY a.attnum
        LOOP
            -- For the CREATE TABLE statement: "colname coltype"
            v_col_def_list := v_col_def_list
                || format('%I %s, ', rec.column_name, rec.full_data_type);

            -- For the INSERT statements: just column names
            v_col_name_list := v_col_name_list
                || format('%I, ', rec.column_name);

            -- For referencing columns in the trigger body
            v_select_list_new := v_select_list_new
                || format('NEW.%I, ', rec.column_name);
            v_select_list_old := v_select_list_old
                || format('OLD.%I, ', rec.column_name);
        END LOOP;

    IF v_col_def_list = '' THEN
        RAISE EXCEPTION 'Table "%" not found or has no columns.', p_table;
    END IF;

    -- Trim trailing comma+space
    v_col_def_list     := rtrim(v_col_def_list, ', ');
    v_col_name_list    := rtrim(v_col_name_list, ', ');
    v_select_list_new  := rtrim(v_select_list_new, ', ');
    v_select_list_old  := rtrim(v_select_list_old, ', ');

    -------------------------------------------------------------------
    -- 3) Create the audit table if it doesn’t exist
    -------------------------------------------------------------------
    EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I.%I (
               %s,
               operation text NOT NULL,
               changed_at timestamptz NOT NULL DEFAULT now()
             )',
            v_schema_name,
            v_audit_name,
            v_col_def_list
            );

    -------------------------------------------------------------------
    -- 4) Create or replace the trigger function
    -------------------------------------------------------------------
    EXECUTE format(
            E'CREATE OR REPLACE FUNCTION %I.%I_audit_fn()\n'||
            E'  RETURNS trigger\n'||
            E'  LANGUAGE plpgsql\n'||
            E'AS $FN$\n'||
            E'BEGIN\n'||
                -- INSERT
            E'  IF (TG_OP = ''INSERT'') THEN\n'||
            E'    INSERT INTO %I.%I (%s, operation)\n'||
            E'    VALUES (%s, ''INSERT'');\n'||
            E'    RETURN NEW;\n'||
                -- UPDATE
            E'  ELSIF (TG_OP = ''UPDATE'') THEN\n'||
            E'    INSERT INTO %I.%I (%s, operation)\n'||
            E'    VALUES (%s, ''UPDATE'');\n'||
            E'    RETURN NEW;\n'||
                -- DELETE
            E'  ELSIF (TG_OP = ''DELETE'') THEN\n'||
            E'    INSERT INTO %I.%I (%s, operation)\n'||
            E'    VALUES (%s, ''DELETE'');\n'||
            E'    RETURN OLD;\n'||
            E'  END IF;\n'||
            E'  RETURN NULL;\n'||
            E'END;\n'||
            E'$FN$;',
        -- placeholders:
        -- 1: schema for function
        -- 2: base table name -> function name
            v_schema_name,
            v_table_name,
        -- 3,4: schema/audit table, 5: col_name_list, 6: NEW references
            v_schema_name,
            v_audit_name,
            v_col_name_list,
            v_select_list_new,
        -- 7,8: schema/audit table, 9: col_name_list, 10: OLD references
            v_schema_name,
            v_audit_name,
            v_col_name_list,
            v_select_list_old,
        -- 11,12: schema/audit table, 13: col_name_list, 14: OLD references
            v_schema_name,
            v_audit_name,
            v_col_name_list,
            v_select_list_old
            );

    -------------------------------------------------------------------
    -- 5) Create or replace the trigger on the original table
    -------------------------------------------------------------------
    EXECUTE format(
            'DROP TRIGGER IF EXISTS %I_audit_trigger ON %I.%I; '||
            'CREATE TRIGGER %I_audit_trigger '||
            'AFTER INSERT OR UPDATE OR DELETE '||
            'ON %I.%I '||
            'FOR EACH ROW '||
            'EXECUTE PROCEDURE %I.%I_audit_fn();',
            v_table_name,
            v_schema_name,
            v_table_name,
            v_table_name,
            v_schema_name,
            v_table_name,
            v_schema_name,
            v_table_name
            );

END;
$$;
-- +migrate StatementEnd


-- `drop_audit_table` removes auditing from a table by dropping the audit table, trigger function, and trigger.
-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION drop_audit_table(p_table text)
    RETURNS void
    LANGUAGE plpgsql
AS $$
DECLARE
    v_schema_name  text;
    v_table_name   text;
    v_audit_name   text;
BEGIN
    -------------------------------------------------------------------
    -- 1) Separate out schema vs. table (default to current_schema if no dot)
    -------------------------------------------------------------------
    IF p_table LIKE '%.%' THEN
        v_schema_name := split_part(p_table, '.', 1);
        v_table_name  := split_part(p_table, '.', 2);
    ELSE
        v_schema_name := current_schema();
        v_table_name  := p_table;
    END IF;

    -------------------------------------------------------------------
    -- 2) Derive the audit table name
    -------------------------------------------------------------------
    v_audit_name := v_table_name || '_audit';

    -------------------------------------------------------------------
    -- 3) Drop the trigger on the original table, if it exists
    -------------------------------------------------------------------
    EXECUTE format(
            'DROP TRIGGER IF EXISTS %I_audit_trigger ON %I.%I;',
            v_table_name,
            v_schema_name,
            v_table_name
            );

    -------------------------------------------------------------------
    -- 4) Drop the audit trigger function, if it exists
    -------------------------------------------------------------------
    EXECUTE format(
            'DROP FUNCTION IF EXISTS %I.%I_audit_fn();',
            v_schema_name,
            v_table_name
            );

    -------------------------------------------------------------------
    -- 5) Drop the audit table, if it exists
    -------------------------------------------------------------------
    EXECUTE format(
            'DROP TABLE IF EXISTS %I.%I;',
            v_schema_name,
            v_audit_name
            );

END;
$$;
-- +migrate StatementEnd

-- +migrate Down
DROP FUNCTION IF EXISTS create_audit_table(p_table text);
DROP FUNCTION IF EXISTS drop_audit_table(p_table text);