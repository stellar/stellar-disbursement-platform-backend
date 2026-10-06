-- Closes a race in the two archive-guard triggers added by 2026-06-09.3 and 2026-06-09.4: both
-- read distribution_wallets.status with a plain (non-locking) SELECT, so a disbursement insert
-- or membership grant running concurrently with an in-flight ArchiveWallet transaction can read
-- the pre-archive status for the full duration of that transaction and succeed against a wallet
-- that is ARCHIVED the instant it commits.
--
-- The FK from disbursements/wallet_memberships to distribution_wallets does not protect against
-- this: it takes an implicit FOR KEY SHARE lock on the referenced row, and per Postgres's
-- row-lock conflict matrix FOR KEY SHARE does not conflict with the FOR NO KEY UPDATE that
-- ArchiveWallet's plain `UPDATE ... SET status = 'ARCHIVED'` takes — so the FK provides
-- referential integrity but no concurrency protection here.
--
-- Fix: add FOR SHARE to both SELECTs. FOR SHARE DOES conflict with FOR NO KEY UPDATE, so a
-- trigger evaluating mid-archive now blocks until the archiving transaction commits or rolls
-- back, then reads the final, authoritative status — closing the window instead of narrowing it.
--
-- Only the function bodies change; the triggers themselves (attachment, timing, table) are
-- untouched, so CREATE OR REPLACE FUNCTION is sufficient.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_disbursement_source_wallet_active()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT status FROM distribution_wallets WHERE id = NEW.source_wallet_id FOR SHARE) = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot create disbursement: source distribution wallet % is archived', NEW.source_wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'disbursements_source_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_wallet_membership_wallet_active()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT status FROM distribution_wallets WHERE id = NEW.wallet_id FOR SHARE) = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot grant membership: distribution wallet % is archived', NEW.wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'wallet_memberships_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate Down

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_disbursement_source_wallet_active()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT status FROM distribution_wallets WHERE id = NEW.source_wallet_id) = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot create disbursement: source distribution wallet % is archived', NEW.source_wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'disbursements_source_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_wallet_membership_wallet_active()
RETURNS TRIGGER AS $$
BEGIN
    IF (SELECT status FROM distribution_wallets WHERE id = NEW.wallet_id) = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot grant membership: distribution wallet % is archived', NEW.wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'wallet_memberships_wallet_must_be_active';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd
