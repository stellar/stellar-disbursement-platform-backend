# Multi-Distribution-Wallet — Cutover Guide

How to roll the multi-wallet migrations onto an existing SDP deployment safely. Existing tenants
are mapped to a single implicit **default** wallet that mirrors their current distribution account,
so single-wallet behavior is preserved and **no key migration is needed** (the legacy account keeps
signing from the existing `vault`). The two things to plan for are (1) an authorization backfill
that must reach every active user, and (2) two migrations that lock large tables.

## What the migrations do to existing data

| Migration | Effect on existing data |
|---|---|
| `.0` create `distribution_wallets` | new table |
| `.1` backfill default wallet | one `is_default` wallet per tenant schema, copied from `admin.tenants` (address/type/status). Idempotent. |
| `.2` `source_wallet_id` on `disbursements` | ADD COLUMN → backfill all rows to the default wallet → `SET NOT NULL` + FK + index |
| `.5` backfill `wallet_memberships` | grants each **non-owner** user a membership on the default wallet for each of their wallet-scopable roles |
| `.6` `source_wallet_id` on `payments` | ADD COLUMN → backfill (inherit disbursement's wallet; direct → default) → `SET NOT NULL` + FK + index + triggers |
| `.3/.4/.7/.8`, tss `.0/.1` | new tables/columns/triggers; copy no existing rows |

After cutover a non-opted-in tenant sees exactly one wallet, all history attributed to it, identical
signing, and wallet-scoped authorization is a no-op — until an Owner creates a second wallet.

## Step 1 — Pre-flight (required)

Run [`preflight.sql`](preflight.sql) against a prod replica. It hard-fails on either hazard:

- **Abort risk:** a schema with disbursements/payments but **no live `admin.tenants` row**
  (missing or soft-deleted), so the default-wallet backfill `.1` never runs and `.2`/`.6` abort at
  `SET NOT NULL`. (The `.2`/`.6` migrations now also RAISE a clear error instead of an opaque
  not-null violation if this slips through.) Fix: ensure the tenant is active (not soft-deleted)
  and has a provisioned distribution account so `.1` creates its default; or exclude the dead
  schema from the run.
- **Lockout risk:** a **genuinely non-owner** user (`is_owner=false` **and** no `owner` role)
  whose roles include none of `financial_controller / developer / business / initiator / approver`.
  The membership backfill grants them nothing, so after cutover every wallet-gated read/write
  returns empty/403 for them. Users with the `owner` role are tenant-wide regardless of the
  `is_owner` flag, so they are **not** at risk. Fix a real case before or right after cutover by
  granting a membership (see Step 4), or confirm those users are inactive.

## Step 2 — Migrate

**Small/medium tenants:** run the normal `db admin|tss|sdp migrate up --all`. `.2`/`.6` complete in
well under a lock window at these sizes.

**Large tenants (millions of `disbursements`/`payments`):** `.2` and `.6` run
`ADD COLUMN → full-table UPDATE → SET NOT NULL → ADD FK → CREATE INDEX` inside **one** transaction,
holding `ACCESS EXCLUSIVE` (a write outage) for the whole rewrite + index build. Either take a
maintenance window, **or** apply the online-safe equivalent below in a low-traffic window and mark
`.2`/`.6` as already applied so the runner skips them.

```sql
-- Per large tenant schema, each statement autocommits (no wrapping txn) so long steps don't hold
-- ACCESS EXCLUSIVE and CREATE INDEX CONCURRENTLY is allowed. Repeat for `payments`.
SET search_path TO sdp_<tenant>;

ALTER TABLE disbursements ADD COLUMN source_wallet_id VARCHAR(36);                         -- fast
-- Backfill; for very large tables do it in batches (e.g. by id range) to bound txn size:
UPDATE disbursements SET source_wallet_id = (SELECT id FROM distribution_wallets WHERE is_default)
  WHERE source_wallet_id IS NULL;                                                          -- row locks only

ALTER TABLE disbursements
  ADD CONSTRAINT chk_disb_src_wallet_nn CHECK (source_wallet_id IS NOT NULL) NOT VALID;    -- brief lock
ALTER TABLE disbursements VALIDATE CONSTRAINT chk_disb_src_wallet_nn;                       -- SHARE UPDATE EXCLUSIVE
ALTER TABLE disbursements ALTER COLUMN source_wallet_id SET NOT NULL;   -- PG12+: uses the validated CHECK, no full scan
ALTER TABLE disbursements DROP CONSTRAINT chk_disb_src_wallet_nn;

ALTER TABLE disbursements ADD CONSTRAINT fk_disbursements_source_wallet_id
  FOREIGN KEY (source_wallet_id) REFERENCES distribution_wallets (id) ON DELETE RESTRICT NOT VALID; -- brief lock
ALTER TABLE disbursements VALIDATE CONSTRAINT fk_disbursements_source_wallet_id;            -- SHARE UPDATE EXCLUSIVE

CREATE INDEX CONCURRENTLY disbursements_source_wallet_id_idx ON disbursements (source_wallet_id);
```

For `payments`, do the same, then also apply the `derive_payment_source_wallet` /
`reject_*_source_wallet_mutation` triggers from `.6`. Finally record both migrations as applied in
the migration-tracking table (e.g. `INSERT INTO gorp_migrations (id, applied_at) VALUES
('2026-06-09.2-add-source-wallet-id-to-disbursements.sql', now()), (...'.6'..., now())`) so
`migrate up` skips them, then run the remaining migrations normally.

Run with `SET lock_timeout = '3s'; SET statement_timeout = '...';` so a blocked DDL step fails fast
instead of queuing behind (and blocking) all traffic. `VACUUM (ANALYZE)` both tables afterward.

## Step 3 — Heads-up for API integrators (behavior cliff)

Once a tenant has **two or more active wallets**, a disbursement/direct-payment create that omits
the `X-Wallet-Id` header returns **`400` ("the X-Wallet-Id header is required")**. This is triggered
by data state, not by the deploy: a single-wallet tenant's existing scripts keep working after the
upgrade, and start failing only when a second wallet is added. Tell integrators to send
`X-Wallet-Id` on all write calls before enabling a second wallet. (Single-wallet tenants are
unaffected — the header stays optional.)

Also note for v1: additional wallets are `DB_VAULT`-type only, and tenants whose existing
distribution account is not `DB_VAULT` cannot create additional wallets.

## Step 4 — Post-migration verification

Run the post-migration query from `preflight.sql` in each tenant schema: any **non-owner** user with
**zero** memberships is locked out. Grant access with:

```
POST /distribution-wallets/{walletId}/memberships   { "user_id": "...", "role": "financial_controller" }
```

Smoke test: an existing single-wallet tenant still lists/creates/starts disbursements exactly as
before (no `X-Wallet-Id` needed), and its distribution account still signs on-chain.

## Rollback

The migration downs drop `source_wallet_id` and the wallet tables. Safe as a **pre-first-write
abort**; once the new code has written any direct payment or created a second wallet, roll forward
rather than back (the `derive_payment_source_wallet` trigger and new rows depend on the columns).
