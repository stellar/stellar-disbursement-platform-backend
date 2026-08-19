-- Give every disbursement an unambiguous source distribution wallet, enforced at the data
-- layer per the spec: source_wallet_id is a NOT NULL foreign key with ON DELETE RESTRICT, so
-- audit-trail integrity is a database invariant — a wallet referenced by any disbursement can
-- never be hard-deleted (archive-don't-delete), and no disbursement can exist without a source.
--
-- Existing disbursements are backfilled to the tenant's default wallet: they were all sent
-- from the tenant's original (single) distribution account, which the default wallet mirrors.

-- +migrate Up

-- Cutover guard: fail fast with a clear message rather than an opaque NOT NULL violation if the
-- default-wallet backfill (2026-06-09.1) did not create a default for a real tenant schema that
-- already holds disbursements (e.g. a soft-deleted-but-lingering tenant, or a schema whose name
-- doesn't match the 'sdp_<tenant>' convention). Layout-aware: only a real per-tenant schema.
-- +migrate StatementBegin
DO $$
BEGIN
    IF left(current_schema()::text, 4) = 'sdp_'
       AND EXISTS (SELECT 1 FROM disbursements)
       AND NOT EXISTS (SELECT 1 FROM distribution_wallets WHERE is_default) THEN
        RAISE EXCEPTION 'multi-wallet cutover: schema % has disbursements but no default distribution wallet; the default-wallet backfill (2026-06-09.1) did not run here. Resolve before backfilling source_wallet_id (see docs/multi-wallet/CUTOVER.md).', current_schema();
    END IF;
END
$$;
-- +migrate StatementEnd

ALTER TABLE disbursements ADD COLUMN source_wallet_id VARCHAR(36);

UPDATE disbursements
SET source_wallet_id = (SELECT id FROM distribution_wallets WHERE is_default)
WHERE source_wallet_id IS NULL;

ALTER TABLE disbursements
    ALTER COLUMN source_wallet_id SET NOT NULL,
    ADD CONSTRAINT fk_disbursements_source_wallet_id
        FOREIGN KEY (source_wallet_id) REFERENCES distribution_wallets (id) ON DELETE RESTRICT;

CREATE INDEX disbursements_source_wallet_id_idx ON disbursements (source_wallet_id);

-- +migrate Down

DROP INDEX disbursements_source_wallet_id_idx;

ALTER TABLE disbursements
    DROP CONSTRAINT fk_disbursements_source_wallet_id,
    DROP COLUMN source_wallet_id;
