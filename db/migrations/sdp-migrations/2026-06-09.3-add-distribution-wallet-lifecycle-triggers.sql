-- DB-layer enforcement for the distribution wallet lifecycle invariants (the API layer
-- enforces the same rules; the spec requires both layers):
--
-- 1. Zero-active-wallets is unreachable: archiving the last ACTIVE wallet is rejected.
--    (Archiving the default directly is already rejected by the
--    distribution_wallets_default_must_be_active CHECK from the table migration.)
-- 2. Archived wallets accept no NEW disbursements, while history stays intact.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_distribution_wallet_not_last_active()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status = 'ACTIVE' AND NEW.status = 'ARCHIVED' THEN
        IF NOT EXISTS (
            SELECT 1 FROM distribution_wallets
            WHERE id != OLD.id AND status = 'ACTIVE'
        ) THEN
            RAISE EXCEPTION 'cannot archive distribution wallet %: tenant must keep at least one active wallet', OLD.id
                USING ERRCODE = 'check_violation', CONSTRAINT = 'distribution_wallets_zero_active_invariant';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER enforce_distribution_wallet_not_last_active_trigger
    BEFORE UPDATE OF status ON distribution_wallets
    FOR EACH ROW EXECUTE PROCEDURE enforce_distribution_wallet_not_last_active();

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_disbursement_source_wallet_active()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT status FROM distribution_wallets WHERE id = NEW.source_wallet_id) = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot create disbursement: source distribution wallet % is archived', NEW.source_wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'disbursements_source_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER enforce_disbursement_source_wallet_active_trigger
    BEFORE INSERT ON disbursements
    FOR EACH ROW EXECUTE PROCEDURE enforce_disbursement_source_wallet_active();

-- +migrate Down

DROP TRIGGER enforce_disbursement_source_wallet_active_trigger ON disbursements;
DROP FUNCTION enforce_disbursement_source_wallet_active;

DROP TRIGGER enforce_distribution_wallet_not_last_active_trigger ON distribution_wallets;
DROP FUNCTION enforce_distribution_wallet_not_last_active;
