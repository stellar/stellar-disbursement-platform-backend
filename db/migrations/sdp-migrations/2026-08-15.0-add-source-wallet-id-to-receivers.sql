-- Give every receiver the distribution wallet it was created under, mirroring source_wallet_id on
-- disbursements (2026-06-09.2) and payments (2026-06-09.6). Visibility is additive: a receiver is
-- reachable from this wallet OR from any wallet that has paid them.
--
-- Existing receivers are backfilled to the tenant's default wallet, which mirrors the tenant's
-- original single distribution account.

-- +migrate Up

-- Cutover guard: fail fast with a clear message rather than an opaque NOT NULL violation if the
-- default-wallet backfill (2026-06-09.1) did not create a default for a real tenant schema that
-- already holds receivers. Layout-aware: only a real per-tenant schema.
-- +migrate StatementBegin
DO $$
BEGIN
    IF left(current_schema()::text, 4) = 'sdp_'
       AND EXISTS (SELECT 1 FROM receivers)
       AND NOT EXISTS (SELECT 1 FROM distribution_wallets WHERE is_default) THEN
        RAISE EXCEPTION 'multi-wallet cutover: schema % has receivers but no default distribution wallet; the default-wallet backfill (2026-06-09.1) did not run here. Resolve before backfilling source_wallet_id.', current_schema();
    END IF;
END
$$;
-- +migrate StatementEnd

ALTER TABLE receivers ADD COLUMN source_wallet_id VARCHAR(36);

UPDATE receivers
SET source_wallet_id = (SELECT id FROM distribution_wallets WHERE is_default)
WHERE source_wallet_id IS NULL;

ALTER TABLE receivers
    ALTER COLUMN source_wallet_id SET NOT NULL,
    ADD CONSTRAINT fk_receivers_source_wallet_id
        FOREIGN KEY (source_wallet_id) REFERENCES distribution_wallets (id) ON DELETE RESTRICT;

CREATE INDEX receivers_source_wallet_id_idx ON receivers (source_wallet_id);

-- receivers_audit already exists, so create_audit_table's CREATE TABLE IF NOT EXISTS will not pick the
-- new column up: add it by hand, then re-run to regenerate the trigger function over all columns.
ALTER TABLE receivers_audit ADD COLUMN source_wallet_id VARCHAR(36);

SELECT create_audit_table('receivers');

-- +migrate Down

DROP INDEX receivers_source_wallet_id_idx;

ALTER TABLE receivers
    DROP CONSTRAINT fk_receivers_source_wallet_id,
    DROP COLUMN source_wallet_id;

ALTER TABLE receivers_audit DROP COLUMN source_wallet_id;

SELECT create_audit_table('receivers');
