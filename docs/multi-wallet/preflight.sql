-- Multi-distribution-wallet cutover pre-flight check.
--
-- Run this against a PRODUCTION replica (or prod, read-only) BEFORE applying the multi-wallet
-- migrations. It scans every not-yet-migrated per-tenant `sdp_<tenant>` schema and hard-fails
-- (RAISE EXCEPTION) if it finds either cutover hazard:
--
--   [ABORT RISK]   a schema that holds disbursements/payments but has NO live `admin.tenants`
--                  row (missing, or soft-deleted). The default-wallet backfill (2026-06-09.1)
--                  only fires for a live tenant, so without it migrations .2/.6 abort at
--                  `SET NOT NULL`. (Note: pre-migration NO schema has a default wallet yet --
--                  that is created *by* .1, so the real question is whether .1 will run.)
--   [LOCKOUT RISK] a genuinely non-owner user (NOT is_owner AND role list does not contain
--                  `owner`) whose roles contain none of the wallet-scopable roles
--                  (financial_controller / developer / business / initiator / approver). The
--                  membership backfill (.5) grants them nothing, and authorization treats them
--                  as non-owners, so they lose all wallet-gated access until an Owner grants a
--                  membership. Users with the `owner` role are tenant-wide regardless of the
--                  is_owner flag, so they are NOT at risk.
--
-- Exit: prints "pre-flight OK" or raises with the count of issues. Resolve every issue before
-- cutover (see docs/multi-wallet/CUTOVER.md). Safe to re-run: already-migrated schemas (those
-- that already have a distribution_wallets table) are skipped.

DO $$
DECLARE
    s          text;
    tenant_nm  text;
    n_data     bigint;
    has_tenant boolean;
    n_lockout  bigint;
    problems   int := 0;
    admin_ok   boolean := to_regclass('admin.tenants') IS NOT NULL;
BEGIN
    IF NOT admin_ok THEN
        RAISE WARNING 'admin.tenants not found -- cannot evaluate ABORT RISK; run this where the admin schema is visible';
    END IF;

    FOR s IN
        SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'sdp\_%'
    LOOP
        -- Not provisioned yet, or already migrated (distribution_wallets exists) -> nothing to check.
        IF to_regclass(s || '.disbursements') IS NULL
           OR to_regclass(s || '.distribution_wallets') IS NOT NULL THEN
            CONTINUE;
        END IF;

        tenant_nm := substring(s from 5);  -- 'sdp_acme' -> 'acme'
        EXECUTE format(
            'SELECT (SELECT count(*) FROM %1$I.disbursements) + '
            '       (SELECT count(*) FROM %1$I.payments)', s) INTO n_data;

        -- ABORT RISK: data present, but .1 will not create a default wallet (no live tenant row).
        IF admin_ok AND n_data > 0 THEN
            SELECT EXISTS (
                SELECT 1 FROM admin.tenants t WHERE t.name = tenant_nm AND t.deleted_at IS NULL
            ) INTO has_tenant;
            IF NOT has_tenant THEN
                RAISE WARNING '[ABORT RISK] %: % disbursement/payment rows but no live admin.tenants row -> default-wallet backfill (.1) will not run -> .2/.6 abort', s, n_data;
                problems := problems + 1;
            END IF;
        END IF;

        -- LOCKOUT RISK: genuinely non-owner users (not is_owner AND no `owner` role) with no
        -- wallet-scopable role.
        IF to_regclass(s || '.auth_users') IS NOT NULL THEN
            EXECUTE format(
                'SELECT count(*) FROM %I.auth_users u '
                'WHERE NOT u.is_owner '
                '  AND NOT (''owner'' = ANY(u.roles)) '
                '  AND NOT (u.roles && ARRAY[''financial_controller'',''developer'',''business'',''initiator'',''approver'']::text[])',
                s) INTO n_lockout;
            IF n_lockout > 0 THEN
                RAISE WARNING '[LOCKOUT RISK] %: % non-owner user(s) with no wallet-scopable role -> zero memberships after cutover', s, n_lockout;
                problems := problems + 1;
            END IF;
        END IF;
    END LOOP;

    IF problems = 0 THEN
        RAISE NOTICE 'pre-flight OK: no abort/lockout risks across not-yet-migrated sdp_ schemas';
    ELSE
        RAISE EXCEPTION 'pre-flight found % issue(s) - resolve before cutover (docs/multi-wallet/CUTOVER.md)', problems;
    END IF;
END $$;

-- ----------------------------------------------------------------------------------------------
-- Per-schema drill-down (run inside a specific tenant schema, e.g. SET search_path TO sdp_acme;)
-- ----------------------------------------------------------------------------------------------

-- Which users would actually be locked out (non-owner by flag AND role, with no scopable role):
--   SELECT id, email, roles FROM auth_users u
--   WHERE NOT u.is_owner
--     AND NOT ('owner' = ANY(u.roles))
--     AND NOT (u.roles && ARRAY['financial_controller','developer','business','initiator','approver']::text[]);

-- POST-migration verification (run AFTER migrating): any genuinely non-owner user with zero
-- memberships is locked out and must be granted access via
-- POST /distribution-wallets/{id}/memberships:
--   SELECT u.id, u.email, u.roles FROM auth_users u
--   WHERE NOT u.is_owner
--     AND NOT ('owner' = ANY(u.roles))
--     AND NOT EXISTS (SELECT 1 FROM wallet_memberships m WHERE m.user_id = u.id);
