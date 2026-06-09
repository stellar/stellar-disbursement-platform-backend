-- Give every disbursement an unambiguous source distribution wallet, enforced at the data
-- layer per the spec: source_wallet_id is a NOT NULL foreign key with ON DELETE RESTRICT, so
-- audit-trail integrity is a database invariant — a wallet referenced by any disbursement can
-- never be hard-deleted (archive-don't-delete), and no disbursement can exist without a source.
--
-- Existing disbursements are backfilled to the tenant's default wallet: they were all sent
-- from the tenant's original (single) distribution account, which the default wallet mirrors.

-- +migrate Up

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
