-- Backfill the implicit "default" distribution wallet for existing tenants: one wallet per
-- tenant schema, pointing at the tenant's current distribution account (admin.tenants), so
-- existing single-wallet tenants observe zero behavior change without opting in.
--
-- Layout-aware: in production every tenant schema is `sdp_<tenant_name>` and the admin schema
-- holds `tenants`. In flat test layouts (all migration domains applied to a single schema,
-- e.g. db/dbtest), `admin.tenants` does not exist and this migration is a safe no-op.
--
-- Tenants whose distribution account is not yet provisioned (e.g. CIRCLE wallets pending user
-- activation) still get their default wallet row, with a NULL address and the tenant's
-- account status, so downstream wallet-membership backfills and FK targets stay consistent.

-- +migrate Up

-- +migrate StatementBegin
DO $$
BEGIN
    IF to_regclass('admin.tenants') IS NOT NULL AND left(current_schema(), 4) = 'sdp_' THEN
        INSERT INTO distribution_wallets
            (name, description, distribution_account_address, distribution_account_type, distribution_account_status, is_default)
        SELECT
            'default',
            'Auto-created wallet for the tenant''s original distribution account.',
            t.distribution_account_address,
            t.distribution_account_type::text,
            t.distribution_account_status::text,
            TRUE
        FROM admin.tenants t
        WHERE ('sdp_' || t.name) = current_schema()
          AND t.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM distribution_wallets WHERE is_default);
    END IF;
END
$$;
-- +migrate StatementEnd

-- +migrate Down

-- Remove only the auto-created default shim row (no-op in flat test layouts).
DELETE FROM distribution_wallets WHERE is_default AND name = 'default';
