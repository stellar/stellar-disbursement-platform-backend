-- The archive guard skipped the one table that actually moves funds. 2026-06-09.3 and 2026-06-09.4
-- stop a disbursement or a membership grant from landing on an ARCHIVED wallet, and 2026-07-30.2
-- made both of those reads lock the wallet row (SELECT ... FOR SHARE) so they serialize against an
-- in-flight archive instead of reading its pre-archive status. payments never got either check: a
-- DIRECT payment states its own source_wallet_id, and nothing in the database has ever looked at
-- that wallet's status.
--
-- The only guard today is resolveSourceWalletForWrite (wallet_scope.go), which reads the wallet on
-- the request's connection and then hands the id to a separate INSERT — exactly the
-- read-outside-the-writing-transaction shape the sibling triggers exist to backstop. An archive
-- committing between those two steps yields a READY direct payment drawing on an archived wallet.
--
-- Scope is direct payments only, and deliberately so. Disbursement payments never state a wallet:
-- they inherit it from their disbursement (derive_payment_source_wallet, 2026-06-09.6), whose own
-- INSERT was already gated by enforce_disbursement_source_wallet_active. Checking them here would
-- close no race — it would instead newly reject an instructions upload onto an existing DRAFT
-- disbursement whose wallet was archived afterwards, which the API allows on purpose
-- (PostDisbursementInstructions authorizes via ensureWalletActionAllowed, which is an entitlement
-- check, not a status check).
--
-- FOR SHARE for the same reason as 2026-07-30.2: it conflicts with the FOR NO KEY UPDATE that
-- ArchiveWallet's plain `UPDATE ... SET status = 'ARCHIVED'` takes, so an insert evaluating
-- mid-archive blocks until that transaction settles and then reads the authoritative status. The
-- FK to distribution_wallets does not help — its implicit FOR KEY SHARE does not conflict with a
-- non-key UPDATE of status.
--
-- Trigger firing order is by name (Postgres runs BEFORE row triggers alphabetically), the same
-- convention 2026-08-07.0 relies on for disbursements: derive_payment_source_wallet_trigger ("d")
-- fills source_wallet_id before enforce_payment_source_wallet_active_trigger ("e") reads it.
--
-- The companion table is resolved via TG_TABLE_SCHEMA rather than the caller's search_path, per
-- 2026-08-07.0: admin.migrate_tenant_data_from_v1_to_v2() writes into a tenant schema that is not
-- on its search_path, and an unqualified name fails there.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_payment_source_wallet_active()
RETURNS TRIGGER AS $$
DECLARE
    wallet_status TEXT;
BEGIN
    IF NEW.disbursement_id IS NOT NULL THEN
        RETURN NEW;
    END IF;

    EXECUTE format(
        'SELECT status FROM %I.distribution_wallets WHERE id = $1 FOR SHARE',
        TG_TABLE_SCHEMA
    ) INTO wallet_status USING NEW.source_wallet_id;

    IF wallet_status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot create direct payment: source distribution wallet % is archived', NEW.source_wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'payments_source_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER enforce_payment_source_wallet_active_trigger
    BEFORE INSERT ON payments
    FOR EACH ROW EXECUTE PROCEDURE enforce_payment_source_wallet_active();

-- +migrate Down

DROP TRIGGER enforce_payment_source_wallet_active_trigger ON payments;
DROP FUNCTION enforce_payment_source_wallet_active;
