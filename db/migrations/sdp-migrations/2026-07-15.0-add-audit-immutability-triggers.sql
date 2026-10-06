-- disbursements_audit and payments_audit are append-only history (2026-06-09.8), but unlike
-- wallet_memberships_audit they had no reject-mutation trigger, so a SQL session could rewrite
-- or erase audit rows. Enforce append-only at the DB layer for both, matching the
-- wallet_memberships_audit protection.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION reject_audit_table_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is append-only: % is not allowed', TG_TABLE_NAME, TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER reject_disbursements_audit_mutation_trigger
    BEFORE UPDATE OR DELETE ON disbursements_audit
    FOR EACH ROW EXECUTE PROCEDURE reject_audit_table_mutation();

CREATE TRIGGER reject_payments_audit_mutation_trigger
    BEFORE UPDATE OR DELETE ON payments_audit
    FOR EACH ROW EXECUTE PROCEDURE reject_audit_table_mutation();

-- +migrate Down

DROP TRIGGER reject_payments_audit_mutation_trigger ON payments_audit;
DROP TRIGGER reject_disbursements_audit_mutation_trigger ON disbursements_audit;
DROP FUNCTION reject_audit_table_mutation;
