-- Backfill wallet memberships: every existing non-owner user becomes a member of their
-- tenant's DEFAULT wallet with their current role(s), so wallet-scoped authorization changes
-- nothing for existing tenants (their only wallet is the default). Owner stays tenant-wide and
-- receives no membership rows.
--
-- Guards make this safe in every layout:
-- - fresh tenant schemas: sdp-migrations run before auth-migrations, so auth_users does not
--   exist yet — no users to backfill (new users get memberships via the management API)
-- - production upgrades: auth_users already exists from prior deploys — backfill runs
-- - re-runs are idempotent (ON CONFLICT DO NOTHING against the unique triple)

-- +migrate Up

-- +migrate StatementBegin
DO $$
BEGIN
    IF to_regclass('auth_users') IS NOT NULL
       AND EXISTS (SELECT 1 FROM distribution_wallets WHERE is_default) THEN
        INSERT INTO wallet_memberships (user_id, wallet_id, role)
        SELECT
            u.id,
            (SELECT id FROM distribution_wallets WHERE is_default),
            r.role
        FROM auth_users u
        CROSS JOIN LATERAL unnest(u.roles) AS r(role)
        WHERE NOT u.is_owner
          AND r.role IN ('financial_controller', 'developer', 'business', 'initiator', 'approver')
        ON CONFLICT (user_id, wallet_id, role) DO NOTHING;
    END IF;
END
$$;
-- +migrate StatementEnd

-- +migrate Down

-- Remove only backfill-shaped rows (default-wallet memberships without an explicit grantor).
DELETE FROM wallet_memberships
WHERE granted_by IS NULL
  AND wallet_id IN (SELECT id FROM distribution_wallets WHERE is_default);
