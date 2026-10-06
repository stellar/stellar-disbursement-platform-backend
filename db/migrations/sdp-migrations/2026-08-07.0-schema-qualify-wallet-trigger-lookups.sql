-- The four wallet trigger functions added by this feature all reach for a second table
-- (distribution_wallets, disbursements) with an UNQUALIFIED name, so the lookup resolves
-- through whatever search_path the writing session happens to have. Every path we exercise in
-- normal operation runs on a tenant connection whose search_path is that tenant's schema, so
-- this looked correct — but it is not, and one supported path already breaks on it.
--
-- admin.migrate_tenant_data_from_v1_to_v2() (admin-migrations/2024-07-15.0) copies a legacy
-- single-tenant database into a tenant schema. It runs on the ADMIN connection and fully
-- qualifies every table itself (INSERT INTO %I.disbursements SELECT * FROM public.disbursements),
-- so the tenant schema is never on the search_path. The moment that INSERT fires
-- enforce_disbursement_source_wallet_active, the trigger body looks up a bare
-- "distribution_wallets" and Postgres raises:
--
--     ERROR: relation "distribution_wallets" does not exist
--     QUERY: (SELECT status FROM distribution_wallets WHERE id = NEW.source_wallet_id FOR SHARE)
--     CONTEXT: PL/pgSQL function "sdp_<tenant>".enforce_disbursement_source_wallet_active()
--
-- That aborts the whole v1->v2 migration, which is what the single-tenant-to-multi-tenant CI job
-- asserts. derive_payment_source_wallet has the same defect one statement later, against
-- "disbursements".
--
-- Fix: resolve the companion table in the SAME schema as the table the trigger fired on, via
-- TG_TABLE_SCHEMA and dynamic SQL. That is correct for any caller regardless of search_path, and
-- keeps working per-tenant without baking a schema name into the function.
--
-- Semantics are otherwise unchanged. In particular the FOR SHARE row locks added by
-- 2026-07-30.2 are preserved verbatim: they close the archive race and dropping them here would
-- silently reintroduce it.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_disbursement_source_wallet_active()
RETURNS TRIGGER AS $$
DECLARE
    wallet_status TEXT;
BEGIN
    EXECUTE format(
        'SELECT status FROM %I.distribution_wallets WHERE id = $1 FOR SHARE',
        TG_TABLE_SCHEMA
    ) INTO wallet_status USING NEW.source_wallet_id;

    IF wallet_status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot create disbursement: source distribution wallet % is archived', NEW.source_wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'disbursements_source_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_wallet_membership_wallet_active()
RETURNS TRIGGER AS $$
DECLARE
    wallet_status TEXT;
BEGIN
    EXECUTE format(
        'SELECT status FROM %I.distribution_wallets WHERE id = $1 FOR SHARE',
        TG_TABLE_SCHEMA
    ) INTO wallet_status USING NEW.wallet_id;

    IF wallet_status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot grant membership: distribution wallet % is archived', NEW.wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'wallet_memberships_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_distribution_wallet_not_last_active()
RETURNS TRIGGER AS $$
DECLARE
    another_active BOOLEAN;
BEGIN
    IF OLD.status = 'ACTIVE' AND NEW.status = 'ARCHIVED' THEN
        EXECUTE format(
            'SELECT EXISTS (SELECT 1 FROM %I.distribution_wallets WHERE id != $1 AND status = ''ACTIVE'')',
            TG_TABLE_SCHEMA
        ) INTO another_active USING OLD.id;

        IF NOT another_active THEN
            RAISE EXCEPTION 'cannot archive distribution wallet %: tenant must keep at least one active wallet', OLD.id
                USING ERRCODE = 'check_violation', CONSTRAINT = 'distribution_wallets_zero_active_invariant';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION derive_payment_source_wallet()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.source_wallet_id IS NULL THEN
        IF NEW.disbursement_id IS NOT NULL THEN
            EXECUTE format(
                'SELECT source_wallet_id FROM %I.disbursements WHERE id = $1',
                TG_TABLE_SCHEMA
            ) INTO NEW.source_wallet_id USING NEW.disbursement_id;
        ELSE
            RAISE EXCEPTION 'direct payments must state their source distribution wallet explicitly'
                USING ERRCODE = 'not_null_violation', CONSTRAINT = 'payments_source_wallet_required';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- Second defect on the same path: disbursements.source_wallet_id is NOT NULL (2026-06-09.2),
-- and unlike payments there is no trigger to derive it. 2026-06-09.2 backfills the rows that
-- exist WHEN THE MIGRATION RUNS, but a legacy row arriving afterwards has no wallet and no way
-- to acquire one:
--
--     INSERT INTO %I.disbursements SELECT * FROM public.disbursements
--     ERROR: null value in column "source_wallet_id" of relation "disbursements"
--            violates not-null constraint
--
-- (`INSERT ... SELECT *` maps positionally, so the trailing v2-only column takes its default,
-- which is NULL.) The API never hits this because resolveSourceWalletForWrite always supplies
-- the column, which is why it went unnoticed.
--
-- Mirror what derive_payment_source_wallet does, and what 2026-06-09.2's own backfill decided
-- for exactly these rows: fall back to the tenant's default wallet. The invariant is unchanged
-- for callers that state a wallet, and the DB stops depending on every writer remembering to.
--
-- Trigger firing order matters and is satisfied by name: Postgres runs BEFORE row triggers in
-- alphabetical order, so derive_disbursement_source_wallet_trigger ("d") fills the column before
-- enforce_disbursement_source_wallet_active_trigger ("e") reads it.

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION derive_disbursement_source_wallet()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.source_wallet_id IS NULL THEN
        EXECUTE format(
            'SELECT id FROM %I.distribution_wallets WHERE is_default',
            TG_TABLE_SCHEMA
        ) INTO NEW.source_wallet_id;

        IF NEW.source_wallet_id IS NULL THEN
            RAISE EXCEPTION 'cannot create disbursement: no default distribution wallet for this tenant'
                USING ERRCODE = 'not_null_violation', CONSTRAINT = 'disbursements_source_wallet_required';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

DROP TRIGGER IF EXISTS derive_disbursement_source_wallet_trigger ON disbursements;
CREATE TRIGGER derive_disbursement_source_wallet_trigger
    BEFORE INSERT ON disbursements
    FOR EACH ROW EXECUTE PROCEDURE derive_disbursement_source_wallet();

-- +migrate Down

DROP TRIGGER IF EXISTS derive_disbursement_source_wallet_trigger ON disbursements;
DROP FUNCTION IF EXISTS derive_disbursement_source_wallet();

-- Restores the search_path-dependent bodies as they stood after 2026-07-30.2.

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_disbursement_source_wallet_active()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT status FROM distribution_wallets WHERE id = NEW.source_wallet_id FOR SHARE) = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot create disbursement: source distribution wallet % is archived', NEW.source_wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'disbursements_source_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_wallet_membership_wallet_active()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT status FROM distribution_wallets WHERE id = NEW.wallet_id FOR SHARE) = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot grant membership: distribution wallet % is archived', NEW.wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'wallet_memberships_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

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

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION derive_payment_source_wallet()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.source_wallet_id IS NULL THEN
        IF NEW.disbursement_id IS NOT NULL THEN
            NEW.source_wallet_id := (SELECT source_wallet_id FROM disbursements WHERE id = NEW.disbursement_id);
        ELSE
            RAISE EXCEPTION 'direct payments must state their source distribution wallet explicitly'
                USING ERRCODE = 'not_null_violation', CONSTRAINT = 'payments_source_wallet_required';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd
