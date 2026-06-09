-- Every payment carries an unambiguous source distribution wallet, completing the W3 routing
-- rule at the data layer:
-- - disbursement payments INHERIT their disbursement's source wallet (derived by trigger, so
--   bulk inserts stay unchanged and can never disagree with their disbursement)
-- - direct payments (no disbursement) must state their source wallet explicitly — the
--   trigger raises if neither a disbursement nor a wallet is provided (no silent defaults)
-- - source_wallet_id is immutable after creation, enforced by an UPDATE-rejection trigger on
--   BOTH payments and disbursements

-- +migrate Up

ALTER TABLE payments ADD COLUMN source_wallet_id VARCHAR(36);

UPDATE payments p
SET source_wallet_id = COALESCE(
    (SELECT d.source_wallet_id FROM disbursements d WHERE d.id = p.disbursement_id),
    (SELECT dw.id FROM distribution_wallets dw WHERE dw.is_default)
)
WHERE p.source_wallet_id IS NULL;

ALTER TABLE payments
    ALTER COLUMN source_wallet_id SET NOT NULL,
    ADD CONSTRAINT fk_payments_source_wallet_id
        FOREIGN KEY (source_wallet_id) REFERENCES distribution_wallets (id) ON DELETE RESTRICT;

CREATE INDEX payments_source_wallet_id_idx ON payments (source_wallet_id);

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION derive_payment_source_wallet()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.source_wallet_id IS NULL THEN
        IF NEW.disbursement_id IS NOT NULL THEN
            NEW.source_wallet_id := (SELECT source_wallet_id FROM disbursements WHERE id = NEW.disbursement_id);
        ELSE
            RAISE EXCEPTION 'direct payments must state their source distribution wallet explicitly'
                USING ERRCODE = 'not_null_violation', CONSTRAINT = 'payments_source_wallet_required';
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER derive_payment_source_wallet_trigger
    BEFORE INSERT ON payments
    FOR EACH ROW EXECUTE PROCEDURE derive_payment_source_wallet();

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION reject_source_wallet_mutation()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.source_wallet_id IS DISTINCT FROM OLD.source_wallet_id THEN
        RAISE EXCEPTION 'source_wallet_id is immutable after creation'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'source_wallet_id_immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

CREATE TRIGGER reject_payment_source_wallet_mutation_trigger
    BEFORE UPDATE OF source_wallet_id ON payments
    FOR EACH ROW EXECUTE PROCEDURE reject_source_wallet_mutation();

CREATE TRIGGER reject_disbursement_source_wallet_mutation_trigger
    BEFORE UPDATE OF source_wallet_id ON disbursements
    FOR EACH ROW EXECUTE PROCEDURE reject_source_wallet_mutation();

-- +migrate Down

DROP TRIGGER reject_disbursement_source_wallet_mutation_trigger ON disbursements;
DROP TRIGGER reject_payment_source_wallet_mutation_trigger ON payments;
DROP FUNCTION reject_source_wallet_mutation;

DROP TRIGGER derive_payment_source_wallet_trigger ON payments;
DROP FUNCTION derive_payment_source_wallet;

DROP INDEX payments_source_wallet_id_idx;

ALTER TABLE payments
    DROP CONSTRAINT fk_payments_source_wallet_id,
    DROP COLUMN source_wallet_id;
