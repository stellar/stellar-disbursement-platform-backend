# Multi-Distribution-Account - Cutover Guide

When you upgrade to 7.0.0, every existing tenant of type `DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT` is mapped to a single **default** distribution account that mirrors the one it already has. Single-account behaviour is preserved, all history is attributed to that account, and no signing key migration is needed; the legacy account keeps signing from the existing `vault`. Nothing changes for a tenant until an Owner adds a second account.

## Before you upgrade (required)

Migrations run per tenant, and only for tenants that are live: soft-deleted and deactivated tenants are skipped, so their schemas keep the old structure and can be migrated later if the tenant is ever restored.

The upgrade will not complete successfully if you have a **live tenant row whose schema is missing**, usually because the schema was dropped manually. DB migrations will fail partway through: tenants processed before it are already migrated and tenants after it are not, so you are left half-migrated. On Helm this fails the init container and the pod crash-loops.

Please verify that the following query returns no rows:

```sql
SELECT t.id, t.name
FROM admin.tenants t
WHERE t.deleted_at IS NULL
  AND t.status != 'TENANT_DEACTIVATED'
  AND to_regnamespace('sdp_' || t.name) IS NULL;
```

For anything it returns, either recreate the missing schema, or take the tenant out of the run by
soft-deleting it:

```sql
UPDATE admin.tenants SET deleted_at = NOW() WHERE id = '<tenant-id>';
```

## After upgrading

Run this once, against the same database. It walks every live tenant and reports anything that
needs attention; nothing silently fails at migration time, so this is what catches the three
states that can be wrong:

```sql
DO $$
DECLARE
    s         text;
    n_default bigint;
    n_users   bigint;
    n_keys    bigint;
    problems  int := 0;
BEGIN
    FOR s IN
        SELECT 'sdp_' || t.name
        FROM admin.tenants t
        WHERE t.deleted_at IS NULL AND t.status != 'TENANT_DEACTIVATED'
        ORDER BY t.name
    LOOP
        IF to_regclass(s || '.distribution_wallets') IS NULL THEN
            RAISE WARNING '%: not migrated', s;
            problems := problems + 1;
            CONTINUE;
        END IF;

        EXECUTE format('SELECT count(*) FROM %I.distribution_wallets WHERE is_default', s)
            INTO n_default;
        IF n_default <> 1 THEN
            RAISE WARNING '%: % default distribution accounts, expected 1', s, n_default;
            problems := problems + 1;
        END IF;

        EXECUTE format($q$
            SELECT count(*) FROM %I.auth_users u
            WHERE NOT u.is_owner
              AND NOT ('owner' = ANY(COALESCE(u.roles, '{}')))
              AND NOT EXISTS (SELECT 1 FROM %I.wallet_memberships m WHERE m.user_id = u.id)
        $q$, s, s) INTO n_users;
        IF n_users > 0 THEN
            RAISE WARNING '%: % user(s) locked out — no account access', s, n_users;
            problems := problems + 1;
        END IF;

        EXECUTE format($q$SELECT count(*) FROM %I.api_keys WHERE distribution_wallet_ids = '{}'$q$, s)
            INTO n_keys;
        IF n_keys > 0 THEN
            RAISE WARNING '%: % API key(s) with no account access', s, n_keys;
            problems := problems + 1;
        END IF;
    END LOOP;

    IF problems = 0 THEN
        RAISE NOTICE 'post-upgrade OK: all live tenants migrated, no lockouts';
    ELSE
        RAISE WARNING 'post-upgrade found % issue(s) — see warnings above', problems;
    END IF;
END $$;
```

It prints `post-upgrade OK`, or a warning per problem:

- **not migrated**: the run stopped before reaching this tenant, so the estate is half-migrated.
  Re-run the migrations.
- **N default distribution accounts, expected 1**: the tenant has no usable default account.
- **user(s) locked out**: they can sign in but see empty lists and 403s. Grant access with
  `POST /distribution-wallets/{walletId}/memberships`.
- **API key(s) with no account access**: the key returns empty lists and 403s on writes.
  Re-scope it with `PATCH /api-keys/{id}`.

Then smoke test that an existing tenant still lists and starts disbursements as before.

## Potential breaking changes

- `PATCH /organization` is now Owner-only. The `financial_controller` role can no longer edit the organization profile.
- TSS Prometheus counters dropped the `event_id`, `tx_id` and `event_time` labels and added `wallet_id`. Those identifiers are now emitted in the logs instead. Update dashboards and alerts that select on them.
- The `*_audit` tables are now append-only; scripts that prune or rewrite them will fail.

For API integrators:
- When authenticating **with an API key**, tenant-wide endpoints (`/organization`, `/organization/circle-config`, `/distribution-wallets`) require the key to be **minted by an Owner**.
- The `X-Wallet-Id` **request header** becomes required when creating disbursements, payments and receivers via direct API call, once a tenant has more than one active distribution account. Existing integrations keep working until a second account is added.

## Rollback: destructive once the new code has run

After upgrading to 7.0.0, rolling back to a previous version is not a clean reversal. The migration downs **drop** the new `source_wallet_id` columns that associate resources with new distribution accounts, and **drop** the `distribution_wallets` and `distribution_wallet_keys` tables.

Rolling back is only safe as an immediate abort, before any tenant uses the multi-wallet feature. At that point
every value in the new columns was copied from data that already existed, so nothing is lost.

After that, it destroys data that exists nowhere else, for example:

- **A direct payment has been created.** `payments.source_wallet_id` is the only record of which distribution account sent it.
  Dropping the column loses that permanently.
- **A second distribution account has been created.** Its private key lives only in `distribution_wallet_keys` (legacy accounts still use the old `vault`). Dropping that table destroys the only copy of the signing key for an account that may hold funds on-chain. It is not recoverable.

Once tenants are using multi-wallet, fix forward with a patch release rather than rolling back.