-- The DB-level zero-active-wallets invariant is racy. enforce_distribution_wallet_not_last_active
-- (2026-06-09.3, schema-qualified by 2026-08-07.0) decides whether a wallet may be archived from a
-- plain, unlocked EXISTS over the other wallets, so two transactions each archiving one of the
-- tenant's last two ACTIVE wallets each read the other as still ACTIVE, and both commit:
--
--     tx1: UPDATE distribution_wallets SET status = 'ARCHIVED' WHERE id = A;  -- sees B ACTIVE, passes
--     tx2: UPDATE distribution_wallets SET status = 'ARCHIVED' WHERE id = B;  -- sees A ACTIVE, passes
--     tx1: COMMIT;   tx2: COMMIT;                                             -- tenant has 0 ACTIVE wallets
--
-- The service layer already serializes its own callers (services.acquireTenantWalletLock takes a
-- per-tenant advisory lock around the read-modify-write in ArchiveWallet). That is exactly why the
-- gap matters: this trigger exists as an INDEPENDENT backstop for the writers that never run
-- through the service — psql sessions, data migrations, ops scripts — and none of them take that
-- lock. A backstop that only holds when the layer above it already held is not a backstop.
--
-- Fix: serialize the check at the database layer too, by taking a transaction-scoped advisory lock
-- keyed on the tenant BEFORE the existence check. The loser of the race blocks until the winner
-- commits (transaction-scoped advisory locks are released at COMMIT/ROLLBACK), re-runs its check
-- against committed state, and is correctly rejected. The re-read genuinely sees the winner's
-- commit: PL/pgSQL functions are VOLATILE, so under READ COMMITTED every statement inside the
-- function body takes a fresh snapshot rather than reusing the one the outer UPDATE started with.
-- READ COMMITTED is what every writer here runs at (the platform never asks for anything else; the
-- only explicit sql.TxOptions in the codebase pin LevelReadCommitted). SERIALIZABLE would also be
-- safe, independently, since SSI sees the read-write dependency cycle this race forms.
--
-- Why an advisory lock and not FOR UPDATE on the other candidate rows: a BEFORE UPDATE row trigger
-- already holds a row lock on its own target when the body runs (the executor locks the tuple to
-- build OLD before firing the trigger), and each archiver would then want a row lock on the
-- complement set — an inverted lock order that deadlocks. Measured, not assumed: with
-- `... WHERE id != $1 AND status = 'ACTIVE' FOR UPDATE` in the body, two concurrent archives of the
-- last two wallets produce
--
--     ERROR: deadlock detected
--     CONTEXT: while locking tuple (0,2) in relation "distribution_wallets"
--              SQL statement "SELECT EXISTS (SELECT 1 FROM ... FOR UPDATE)"
--
-- while the advisory lock returns the invariant violation. Row locks would also be too broad:
-- FOR UPDATE on every other wallet conflicts with the FOR SHARE that the sibling guards
-- (enforce_disbursement_source_wallet_active, enforce_payment_source_wallet_active,
-- enforce_wallet_membership_wallet_active) take, and with the FOR KEY SHARE the FKs take, so
-- archiving one wallet would block disbursement, payment and membership writes against every
-- OTHER wallet in the tenant. The advisory lock touches none of that.
--
-- Lock ordering, so this cannot deadlock against those siblings (2026-07-30.2, carried through
-- 2026-08-07.0 and 2026-08-07.2): they take FOR SHARE on exactly one wallet row and never take
-- this advisory lock, and this trigger takes no row locks beyond the one the executor already
-- holds on its own target. An archiver therefore waits only on the advisory lock, and it waits on
-- it while holding nothing that another archiver wants, so no waits-for cycle can form. The
-- reverse direction is unchanged: a guard's FOR SHARE still blocks on an uncommitted archive of
-- the row it is guarding, which is the race 2026-07-30.2 closed.
--
-- Key choice. The two-int32 form pg_advisory_xact_lock(int, int) occupies a different advisory
-- lock space from the single-bigint form (pg_locks.objsubid 2 vs 1), so this lock provably cannot
-- collide with the service's pg_advisory_xact_lock(hashtext(current_schema())::bigint) or with the
-- TSS pg_try_advisory_lock(bigint), even on a hash collision. Keeping it separate is deliberate:
-- sharing the service's key would mean a transaction holding that key and then taking a wallet row
-- lock (PromoteToDefault, ArchiveWallet) could deadlock against a direct-SQL archiver holding that
-- same row and waiting for the key inside this trigger.
--
-- The tenant is identified by TG_TABLE_SCHEMA, not current_schema(), for the same reason
-- 2026-08-07.0 schema-qualified the lookups: the trigger must resolve the tenant explicitly rather
-- than through the caller's search_path. Distinct tenants hash to distinct keys and do not
-- serialize against each other.
--
-- Body is otherwise identical to 2026-08-07.0: same ACTIVE -> ARCHIVED condition, same
-- schema-qualified EXECUTE format lookup, same error message, code and constraint name.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_distribution_wallet_not_last_active()
RETURNS TRIGGER AS $$
DECLARE
    another_active BOOLEAN;
BEGIN
    IF OLD.status = 'ACTIVE' AND NEW.status = 'ARCHIVED' THEN
        -- Serialize concurrent archives within this tenant. Held until COMMIT/ROLLBACK, so the
        -- check below and the UPDATE it authorizes are one critical section.
        PERFORM pg_advisory_xact_lock(
            hashtext('distribution_wallets_zero_active_invariant'),
            hashtext(TG_TABLE_SCHEMA::text)
        );

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

-- +migrate Down

-- Restores the 2026-08-07.0 body: schema-qualified, but with the unserialized check.

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
