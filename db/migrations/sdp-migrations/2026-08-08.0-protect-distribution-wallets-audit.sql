-- distribution_wallets_audit (2026-06-09.0) was left mutable: create_audit_table records every
-- change but does not reject one, and 2026-07-15.0 only covered the two tables 2026-06-09.8 had
-- just added. That made wallet lifecycle history — create, archive, promote-to-default, address
-- assignment — the one audit trail in this feature a SQL session could silently rewrite, against
-- the append-only guarantee the rest of the audit tables already enforce.
--
-- reject_audit_table_mutation() belongs to 2026-07-15.0, which always applies first and drops the
-- function in its own Down, so this migration only owns the trigger.

-- +migrate Up

CREATE TRIGGER reject_distribution_wallets_audit_mutation_trigger
    BEFORE UPDATE OR DELETE ON distribution_wallets_audit
    FOR EACH ROW EXECUTE PROCEDURE reject_audit_table_mutation();

-- +migrate Down

DROP TRIGGER reject_distribution_wallets_audit_mutation_trigger ON distribution_wallets_audit;
