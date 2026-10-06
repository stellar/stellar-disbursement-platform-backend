-- Wallet-scoped authorization: wallet_memberships records (user, wallet, role), layered on the
-- existing tenant-level role storage. Owner is always tenant-wide and therefore never holds
-- membership rows; every other role is wallet-scoped. Role SEMANTICS are unchanged — this is
-- an extra scoping dimension, not a redesign.
--
-- Lives in sdp-migrations (it is an SDP authorization concern): user_id/granted_by reference
-- auth_users LOGICALLY (no FK) because the auth domain migrates independently of SDP and the
-- standalone stellar-auth library must not depend on SDP tables. The wallet FK stays — both
-- tables share the SDP domain.
--
-- DB-enforced rules (the API layer enforces the same, with friendlier errors):
-- - new grants on ARCHIVED wallets are rejected (the API maps this to 409); existing
--   memberships persist through archival for historical authorization queries
-- - membership audit entries are immutable and append-only (UPDATE/DELETE rejected)
-- - a wallet referenced by memberships cannot be hard-deleted (consistent with
--   archive-don't-delete)

-- +migrate Up

CREATE TABLE wallet_memberships (
    id VARCHAR(36) PRIMARY KEY DEFAULT public.uuid_generate_v4(),
    user_id VARCHAR(36) NOT NULL,
    wallet_id VARCHAR(36) NOT NULL REFERENCES distribution_wallets (id) ON DELETE RESTRICT,
    role VARCHAR(32) NOT NULL
        CHECK (role IN ('financial_controller', 'developer', 'business', 'initiator', 'approver')),
    granted_by VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, wallet_id, role)
);

CREATE INDEX wallet_memberships_user_id_idx ON wallet_memberships (user_id);
CREATE INDEX wallet_memberships_wallet_id_idx ON wallet_memberships (wallet_id);

-- TRIGGER: updated_at
CREATE TRIGGER refresh_wallet_memberships_updated_at
    BEFORE UPDATE ON wallet_memberships
    FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- New grants on archived wallets are rejected (inert grants are audit-log noise).
-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_wallet_membership_wallet_active()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT status FROM distribution_wallets WHERE id = NEW.wallet_id) = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot grant membership: distribution wallet % is archived', NEW.wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'wallet_memberships_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER enforce_wallet_membership_wallet_active_trigger
    BEFORE INSERT ON wallet_memberships
    FOR EACH ROW EXECUTE PROCEDURE enforce_wallet_membership_wallet_active();

-- Immutable history of every membership change (grant + revoke), append-only.
SELECT create_audit_table('wallet_memberships');

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION reject_wallet_memberships_audit_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'wallet_memberships_audit is append-only: % is not allowed', TG_OP
        USING ERRCODE = 'insufficient_privilege';
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER reject_wallet_memberships_audit_mutation_trigger
    BEFORE UPDATE OR DELETE ON wallet_memberships_audit
    FOR EACH ROW EXECUTE PROCEDURE reject_wallet_memberships_audit_mutation();

-- +migrate Down

DROP TRIGGER reject_wallet_memberships_audit_mutation_trigger ON wallet_memberships_audit;
DROP FUNCTION reject_wallet_memberships_audit_mutation;

SELECT drop_audit_table('wallet_memberships');

DROP TRIGGER enforce_wallet_membership_wallet_active_trigger ON wallet_memberships;
DROP FUNCTION enforce_wallet_membership_wallet_active;

DROP TRIGGER refresh_wallet_memberships_updated_at ON wallet_memberships;

DROP TABLE wallet_memberships;
