-- Multi-distribution-wallet cutover pre-flight check.
--
-- Run this against a PRODUCTION replica (or prod, read-only) BEFORE applying the multi-wallet
-- migrations. It scans every per-tenant `sdp_<tenant>` schema and hard-fails (RAISE EXCEPTION)
-- if it finds either cutover hazard:
--
--   [ABORT RISK]   a schema that holds disbursements/payments but has no default distribution
--                  wallet -> migrations 2026-06-09.2 / .6 will abort at `SET NOT NULL`.
--   [LOCKOUT RISK] a non-owner user whose roles contain none of the wallet-scopable roles
--                  (financial_controller / developer / business / initiator / approver) ->
--                  the membership backfill (.5) grants them nothing, so they lose all
--                  wallet-gated access after cutover until an Owner grants them a membership.
--
-- Exit: prints "pre-flight OK" or raises with the count of issues. Resolve every issue before
-- cutover (see docs/multi-wallet/CUTOVER.md).

DO $$
DECLARE
    s          text;
    n_data     bigint;
    n_default  bigint;
    n_lockout  bigint;
    problems   int := 0;
BEGIN
    FOR s IN
        SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'sdp\_%'
    LOOP
        -- Skip a schema that isn't fully provisioned (missing the tables we need).
        IF to_regclass(s || '.disbursements') IS NULL
           OR to_regclass(s || '.distribution_wallets') IS NULL THEN
            CONTINUE;
        END IF;

        EXECUTE format(
            'SELECT (SELECT count(*) FROM %1$I.disbursements) + '
            '       (SELECT count(*) FROM %1$I.payments)', s) INTO n_data;
        EXECUTE format('SELECT count(*) FROM %I.distribution_wallets WHERE is_default', s)
            INTO n_default;

        IF n_data > 0 AND n_default = 0 THEN
            RAISE WARNING '[ABORT RISK] %: % disbursement/payment rows but no default wallet -> .2/.6 will fail', s, n_data;
            problems := problems + 1;
        END IF;

        IF to_regclass(s || '.auth_users') IS NOT NULL THEN
            EXECUTE format(
                'SELECT count(*) FROM %I.auth_users u WHERE NOT u.is_owner '
                'AND NOT (u.roles && ARRAY[''financial_controller'',''developer'',''business'',''initiator'',''approver'']::text[])',
                s) INTO n_lockout;
            IF n_lockout > 0 THEN
                RAISE WARNING '[LOCKOUT RISK] %: % non-owner user(s) with no wallet-scopable role -> zero memberships after cutover', s, n_lockout;
                problems := problems + 1;
            END IF;
        END IF;
    END LOOP;

    IF problems = 0 THEN
        RAISE NOTICE 'pre-flight OK: no abort/lockout risks across sdp_ schemas';
    ELSE
        RAISE EXCEPTION 'pre-flight found % issue(s) - resolve before cutover (docs/multi-wallet/CUTOVER.md)', problems;
    END IF;
END $$;

-- ----------------------------------------------------------------------------------------------
-- Per-schema drill-down (run inside a specific tenant schema, e.g. SET search_path TO sdp_acme;)
-- ----------------------------------------------------------------------------------------------

-- Which non-owner users would be locked out (no wallet-scopable role):
--   SELECT id, email, roles FROM auth_users u
--   WHERE NOT u.is_owner
--     AND NOT (u.roles && ARRAY['financial_controller','developer','business','initiator','approver']::text[]);

-- POST-migration verification (run AFTER migrating): any non-owner with zero memberships is
-- locked out and must be granted access via POST /distribution-wallets/{id}/memberships:
--   SELECT u.id, u.email, u.roles FROM auth_users u
--   WHERE NOT u.is_owner
--     AND NOT EXISTS (SELECT 1 FROM wallet_memberships m WHERE m.user_id = u.id);
