-- Per-wallet secret storage for multi-distribution-wallet support, using envelope encryption:
-- each wallet's private key is encrypted with its own random data-encryption key (DEK), and the
-- DEK is wrapped by the host-level distribution-account passphrase (KEK). Rotating one wallet's
-- DEK re-encrypts only that wallet's row; sibling wallets' ciphertext is never touched, and a
-- corrupted or unavailable key fails closed for that wallet alone.
--
-- Legacy single-wallet tenants keep their secrets in the existing `vault` table — untouched.

-- +migrate Up

CREATE TABLE distribution_wallet_keys (
    public_key VARCHAR(64) PRIMARY KEY,
    encrypted_private_key VARCHAR(256) NOT NULL,
    encrypted_dek VARCHAR(256) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- TRIGGER: updated_at
CREATE TRIGGER refresh_distribution_wallet_keys_updated_at
    BEFORE UPDATE ON distribution_wallet_keys
    FOR EACH ROW EXECUTE PROCEDURE update_at_refresh();

-- +migrate Down

DROP TRIGGER refresh_distribution_wallet_keys_updated_at ON distribution_wallet_keys;

DROP TABLE distribution_wallet_keys;
