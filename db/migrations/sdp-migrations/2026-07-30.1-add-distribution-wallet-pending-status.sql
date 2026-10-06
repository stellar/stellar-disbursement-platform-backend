-- Adds PENDING to distribution_wallets.status: CreateWallet reserves the row before the on-chain
-- keypair is generated and funded, and previously had no way to mark that row as not-yet-usable
-- — it landed as ACTIVE with a NULL address, indistinguishable from a working wallet for the
-- whole provisioning window. See distribution_wallet_management_service.go CreateWallet, which
-- now inserts PENDING and promotes to ACTIVE only once CreateAndFundAccount succeeds (mirroring
-- tenant provisioning's TENANT_CREATED -> TENANT_PROVISIONED shape, pkg/schema/tenant.go).

-- +migrate Up
ALTER TABLE distribution_wallets DROP CONSTRAINT distribution_wallets_status_check;
ALTER TABLE distribution_wallets ADD CONSTRAINT distribution_wallets_status_check
    CHECK (status IN ('ACTIVE', 'ARCHIVED', 'PENDING'));

-- +migrate Down
ALTER TABLE distribution_wallets DROP CONSTRAINT distribution_wallets_status_check;
ALTER TABLE distribution_wallets ADD CONSTRAINT distribution_wallets_status_check
    CHECK (status IN ('ACTIVE', 'ARCHIVED'));
