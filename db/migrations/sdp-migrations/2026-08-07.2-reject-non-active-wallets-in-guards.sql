-- The wallet-active guards reject only ARCHIVED, so a PENDING wallet passes them.
--
-- PENDING was added by 2026-07-30.1 for exactly the window these guards are meant to cover:
-- CreateWallet reserves the row before the keypair is generated and funded, so a PENDING wallet
-- has a NULL distribution_account_address and no usable account behind it. Admitting one lets a
-- disbursement or a membership be created against a wallet that cannot send anything, and the
-- failure surfaces much later and much less legibly than a rejection here would.
--
-- The constraint is named must_be_active and the functions are named *_wallet_active; the check
-- should mean what those names say. Comparing against ACTIVE rather than enumerating the bad
-- states also means any status added later is rejected by default, which is the safe direction --
-- 2026-07-30.1 added PENDING and left these guards untouched, which is how this gap opened.
--
-- Deliberately NOT tightened: enforce_wallet_membership_wallet_active still rejects only ARCHIVED.
-- Membership is access administration, not value movement — granting a team access to a wallet
-- that is still provisioning is legitimate (CreateWallet inserts PENDING and promotes to ACTIVE
-- moments later), and the disbursement and payment guards below are what actually stop an
-- unfunded wallet from being spent from. Blocking the grant would add friction with no security
-- benefit; see the "pending (unfunded, mid-provisioning) wallet" case in wallet_routing_test.go,
-- which grants exactly this membership in order to assert the write path rejects the wallet.
--
-- Bodies otherwise identical to 2026-08-07.0 (schema-qualified via EXECUTE format so they resolve
-- tenant tables explicitly rather than through the caller's search_path), and the FOR SHARE row
-- lock from 2026-07-30.2 is preserved so an in-flight archive still serializes against these.

-- +migrate Up

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_disbursement_source_wallet_active()
RETURNS TRIGGER AS $$
DECLARE
    wallet_status TEXT;
BEGIN
    EXECUTE format(
        'SELECT status FROM %I.distribution_wallets WHERE id = $1 FOR SHARE',
        TG_TABLE_SCHEMA
    ) INTO wallet_status USING NEW.source_wallet_id;

    IF wallet_status IS DISTINCT FROM 'ACTIVE' THEN
        RAISE EXCEPTION 'cannot create disbursement: source distribution wallet % is not active (status: %)', NEW.source_wallet_id, COALESCE(wallet_status, 'not found')
            USING ERRCODE = 'check_violation', CONSTRAINT = 'must_be_active';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_payment_source_wallet_active()
RETURNS TRIGGER AS $$
DECLARE
    wallet_status TEXT;
BEGIN
    -- Disbursement payments inherit their parent's wallet, which was already gated when the
    -- disbursement was created; re-checking here would reject legitimate inserts against a
    -- wallet archived after the fact. Direct payments have no parent, so this is their only
    -- database-level guard. (Carried over from 2026-08-07.1 unchanged.)
    IF NEW.disbursement_id IS NOT NULL THEN
        RETURN NEW;
    END IF;

    EXECUTE format(
        'SELECT status FROM %I.distribution_wallets WHERE id = $1 FOR SHARE',
        TG_TABLE_SCHEMA
    ) INTO wallet_status USING NEW.source_wallet_id;

    IF wallet_status IS DISTINCT FROM 'ACTIVE' THEN
        RAISE EXCEPTION 'cannot create direct payment: source distribution wallet % is not active (status: %)', NEW.source_wallet_id, COALESCE(wallet_status, 'not found')
            USING ERRCODE = 'check_violation', CONSTRAINT = 'payments_source_wallet_must_be_active';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate Down

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_disbursement_source_wallet_active()
RETURNS TRIGGER AS $$
DECLARE
    wallet_status TEXT;
BEGIN
    EXECUTE format(
        'SELECT status FROM %I.distribution_wallets WHERE id = $1 FOR SHARE',
        TG_TABLE_SCHEMA
    ) INTO wallet_status USING NEW.source_wallet_id;

    IF wallet_status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot create disbursement: source distribution wallet % is archived', NEW.source_wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'must_be_active';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd

-- +migrate StatementBegin
CREATE OR REPLACE FUNCTION enforce_payment_source_wallet_active()
RETURNS TRIGGER AS $$
DECLARE
    wallet_status TEXT;
BEGIN
    IF NEW.disbursement_id IS NOT NULL THEN
        RETURN NEW;
    END IF;

    EXECUTE format(
        'SELECT status FROM %I.distribution_wallets WHERE id = $1 FOR SHARE',
        TG_TABLE_SCHEMA
    ) INTO wallet_status USING NEW.source_wallet_id;

    IF wallet_status = 'ARCHIVED' THEN
        RAISE EXCEPTION 'cannot create direct payment: source distribution wallet % is archived', NEW.source_wallet_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'payments_source_wallet_must_be_active';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +migrate StatementEnd
