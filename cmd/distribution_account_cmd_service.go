package cmd

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

//go:generate mockery --name=DistAccCmdServiceInterface --case=underscore --structname=MockDistAccCmdServiceInterface --inpackage --filename=dist_acc_cmd_service_mock.go
type DistAccCmdServiceInterface interface {
	RotateDistributionAccount(ctx context.Context, distAccService DistributionAccountService) error
}

type DistAccCmdService struct{}

func (d *DistAccCmdService) RotateDistributionAccount(ctx context.Context, distAccService DistributionAccountService) error {
	return distAccService.rotateDistributionAccount(ctx)
}

var _ DistAccCmdServiceInterface = (*DistAccCmdService)(nil)

type DistributionAccountService struct {
	distAccService             services.DistributionAccountServiceInterface
	submitterEngine            engine.SubmitterEngine
	tenantManager              tenant.ManagerInterface
	maxBaseFee                 int64
	nativeAssetBootstrapAmount int

	// models is scoped to the TENANT schema, which owns distribution_wallets. Rotation must
	// leave that row pointing at the account it just created, so this is required for every
	// rotation — including the default one, whose wallet row used to be left stale.
	models *data.Models
	// walletKeyService owns per-wallet envelope-encrypted key material
	// (distribution_wallet_keys, per-wallet DEK wrapped by the host KEK). Required to rotate a
	// non-default wallet; the default wallet's key lives in the legacy vault instead.
	walletKeyService tssSvc.DistributionWalletKeyServiceInterface
	// walletID selects which distribution wallet to rotate. Empty means the tenant's default
	// wallet, i.e. the tenant distribution account.
	walletID string
}

// rotateDistributionAccount rotates a distribution account by creating a new account from the old account,
// funding the new account, adding trustlines to the new account, transferring assets to the new account,
// removing trustlines from the old account and merging the old account into the new account.
//
// Without --wallet-id it rotates the tenant's default distribution account. With --wallet-id it
// rotates that wallet instead, minting the replacement keypair into distribution_wallet_keys
// under its own DEK.
//
// In both cases the DB row that names the live account is repointed BEFORE the superseded key is
// deleted: the inconsistency window opens the moment the merge confirms on-chain, so a failure
// after that point must leave the DB naming an account whose key still exists. Both writes are
// idempotent, so a failed run can simply be re-run.
func (s *DistributionAccountService) rotateDistributionAccount(ctx context.Context) error {
	// Every rotation ends by repointing a distribution_wallets row, so fail before anything moves
	// on-chain if the tenant schema is not reachable.
	if s.models == nil {
		return fmt.Errorf("rotating distribution account: tenant schema models are not configured")
	}

	// 1. Get the current distribution account and check if it's a Stellar DB Vault account
	tenantAccount, err := s.submitterEngine.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}
	if tenantAccount.Type != schema.DistributionAccountStellarDBVault {
		return fmt.Errorf("distribution account rotation is only supported for Stellar DB Vault accounts")
	}

	// 2. Resolve the wallet to rotate. The default wallet IS the tenant distribution account —
	// its key lives in the vault and the tenant row must move with it — so naming it explicitly
	// takes the same path as passing no --wallet-id at all.
	if s.walletID != "" {
		wallet, wErr := s.resolveWalletToRotate(ctx)
		if wErr != nil {
			return wErr
		}
		if !wallet.IsDefault {
			return s.rotateWalletAccount(ctx, wallet)
		}
	}

	return s.rotateDefaultAccount(ctx, tenantAccount)
}

// resolveWalletToRotate loads and validates the wallet named by --wallet-id. It mirrors the
// eligibility rules CreateWallet applies: only STELLAR.DB_VAULT wallets are handled in v1, and
// only a live (ACTIVE, provisioned) wallet can be rotated — a PENDING wallet has no account to
// merge, and an ARCHIVED one must not be brought back on-chain.
func (s *DistributionAccountService) resolveWalletToRotate(ctx context.Context) (*data.DistributionWallet, error) {
	wallet, err := s.models.DistributionWallets.Get(ctx, s.models.DBConnectionPool, s.walletID)
	if err != nil {
		return nil, fmt.Errorf("getting distribution wallet %q: %w", s.walletID, err)
	}
	if wallet.Status != data.ActiveDistributionWalletStatus {
		return nil, fmt.Errorf("distribution wallet %q is %s: only ACTIVE distribution wallets can be rotated", wallet.ID, wallet.Status)
	}
	if wallet.AccountType != schema.DistributionAccountStellarDBVault {
		return nil, fmt.Errorf("distribution wallet %q has account type %q: rotation is only supported for %s wallets", wallet.ID, wallet.AccountType, schema.DistributionAccountStellarDBVault)
	}
	if wallet.Address == nil || *wallet.Address == "" {
		return nil, fmt.Errorf("distribution wallet %q has no distribution account address to rotate", wallet.ID)
	}

	return wallet, nil
}

// rotateDefaultAccount rotates the tenant's default distribution account: a fresh keypair is
// minted into the vault, the old account is merged into it, and both rows that name the tenant's
// account — the tenant row (admin schema) and the default distribution_wallets row (tenant
// schema) — are repointed before the old key is deleted.
func (s *DistributionAccountService) rotateDefaultAccount(ctx context.Context, oldAccount schema.TransactionAccount) error {
	// 1. Create a new Stellar account from the old account
	newAccount, err := s.createNewStellarAccountFromAccount(ctx, oldAccount)
	if err != nil {
		return fmt.Errorf("creating new account: %w", err)
	}

	// 2. Update the tenant with the new distribution account
	t, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil {
		return fmt.Errorf("couldn't get tenant from context: %w", err)
	}

	_, err = s.tenantManager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
		ID:                         t.ID,
		DistributionAccountAddress: newAccount.Address,
	})
	if err != nil {
		return fmt.Errorf("updating tenant distribution account: %w", err)
	}

	// 3. Point the default distribution wallet row at the new account too — leaving it on the
	// merged-away account is the multi-wallet half of this rotation. The tenant row (admin
	// schema) and distribution_wallets (tenant schema) sit behind two separate pools, so this
	// cannot be one transaction. The tenant row is written first on purpose: a failure here
	// leaves the authoritative account resolvable, so re-running the command rotates from the
	// live account and completes the sync. EnsureDefaultWallet is an idempotent upsert on
	// WHERE is_default, so the re-run is safe.
	if err = s.syncDefaultWalletRow(ctx, *newAccount); err != nil {
		return fmt.Errorf("syncing default distribution wallet: %w", err)
	}

	// 4. Delete the old account
	if err = s.submitterEngine.Delete(ctx, oldAccount); err != nil {
		return fmt.Errorf("deleting old account: %w", err)
	}

	return nil
}

// syncDefaultWalletRow makes the tenant schema's default distribution wallet row mirror the
// rotated account (address, type and status).
func (s *DistributionAccountService) syncDefaultWalletRow(ctx context.Context, newAccount schema.TransactionAccount) error {
	address := newAccount.Address
	if _, err := s.models.DistributionWallets.EnsureDefaultWallet(ctx, s.models.DBConnectionPool, &address, newAccount.Type, newAccount.Status); err != nil {
		return fmt.Errorf("ensuring default distribution wallet points at %q: %w", address, err)
	}

	log.Ctx(ctx).Infof("🔗 Default distribution wallet now points at account %q", address)

	return nil
}

// rotateWalletAccount rotates ONE non-default distribution wallet: it mints a fresh keypair into
// distribution_wallet_keys under its own DEK (exactly as
// DistributionWalletManagementService.CreateWallet does), merges the wallet's current account
// into it, repoints the wallet row, and only then deletes the superseded key. Sibling wallets and
// the tenant's distribution-account fields are never touched.
func (s *DistributionAccountService) rotateWalletAccount(ctx context.Context, wallet *data.DistributionWallet) error {
	if s.walletKeyService == nil {
		return fmt.Errorf("rotating distribution wallet %q: wallet key service is not configured", wallet.ID)
	}

	oldAccount := schema.TransactionAccount{
		Address: *wallet.Address,
		Type:    wallet.AccountType,
		Status:  wallet.AccountStatus,
	}

	// 1. Mint the replacement keypair with its own DEK.
	log.Ctx(ctx).Infof("🔨 Creating new distribution account for wallet %q from account %q", wallet.ID, oldAccount.Address)
	kp, err := keypair.Random()
	if err != nil {
		return fmt.Errorf("creating new account: generating keypair for distribution wallet %q: %w", wallet.ID, err)
	}
	if err = s.walletKeyService.StoreKey(ctx, kp.Address(), kp.Seed()); err != nil {
		return fmt.Errorf("creating new account: storing key for distribution wallet %q: %w", wallet.ID, err)
	}
	newAccount := schema.TransactionAccount{
		Address: kp.Address(),
		Type:    wallet.AccountType,
		Status:  wallet.AccountStatus,
	}

	// 2. Merge the wallet's current account into the new one.
	if err = s.migrateAccount(ctx, oldAccount, newAccount); err != nil {
		// The new key is deliberately left in place. The merge may have confirmed on-chain even
		// though we failed to observe it, in which case the wallet's funds are already on the
		// new account and deleting its key would destroy them. An unused key row is harmless.
		log.Ctx(ctx).Errorf("rotation of distribution wallet %q failed after minting key %s; the key was kept in case the merge confirmed on-chain — verify the account before discarding it", wallet.ID, newAccount.Address)
		return fmt.Errorf("creating new account: %w", err)
	}

	// 3. Point the wallet row at the new account BEFORE the old key is deleted, so a failure at
	// step 4 leaves the row naming an account we can still sign for. The write is idempotent.
	if _, err = s.models.DistributionWallets.SetRotatedAddress(ctx, s.models.DBConnectionPool, wallet.ID, newAccount.Address); err != nil {
		return fmt.Errorf("pointing distribution wallet %q at rotated account %q: %w", wallet.ID, newAccount.Address, err)
	}

	// 4. Delete the old account's key.
	if err = s.walletKeyService.DeleteKey(ctx, oldAccount.Address); err != nil {
		return fmt.Errorf("deleting old account: %w", err)
	}

	return nil
}

func (s *DistributionAccountService) createNewStellarAccountFromAccount(ctx context.Context, oldAccount schema.TransactionAccount) (*schema.TransactionAccount, error) {
	// 1. Create new account and persist it
	log.Ctx(ctx).Infof("🔨 Creating new distribution account from account %q", oldAccount.Address)
	newAccounts, err := s.submitterEngine.BatchInsert(ctx, schema.DistributionAccountStellarDBVault, 1)
	if err != nil {
		return nil, fmt.Errorf("inserting new account: %w", err)
	}
	if len(newAccounts) != 1 {
		return nil, fmt.Errorf("expected 1 new account, got %d", len(newAccounts))
	}
	newAccount := newAccounts[0]

	if err = s.migrateAccount(ctx, oldAccount, newAccount); err != nil {
		return nil, err
	}

	return &newAccount, nil
}

// migrateAccount funds newAccount out of oldAccount, recreates oldAccount's trustlines and
// balances on it, and merges oldAccount into it — all in a single Stellar transaction. It is
// agnostic about where either account's key material is stored.
func (s *DistributionAccountService) migrateAccount(ctx context.Context, oldAccount, newAccount schema.TransactionAccount) error {
	// 1. Get old account balances to create trustlines on the new account
	oldAccBalances, err := s.distAccService.GetBalances(ctx, &oldAccount)
	if err != nil {
		return fmt.Errorf("getting old account balances: %w", err)
	}

	// 2. Build operations to create new account and merge old account
	var operations []txnbuild.Operation
	numberOfTrustlines := len(oldAccBalances) - 1 // Subtract 1 for the native asset
	operations = append(operations, s.buildCreateAccountOperation(ctx, newAccount, numberOfTrustlines))
	operations = append(operations, s.buildMergeAccountsOperations(ctx, newAccount, oldAccount, oldAccBalances)...)

	// 3. Build and submit the transaction.
	if err = s.buildAndSubmitTx(ctx, oldAccount, newAccount, operations); err != nil {
		return fmt.Errorf("building and signing transaction: %w", err)
	}

	return nil
}

func (s *DistributionAccountService) buildCreateAccountOperation(ctx context.Context, newAccount schema.TransactionAccount, numberOfTrustlines int) txnbuild.Operation {
	baseReserveFee := decimal.NewFromFloat(0.5)
	baseReserves := decimal.NewFromInt(2).Mul(baseReserveFee)                              // 2 base reserves to exist on the ledger
	trustlineReserves := decimal.NewFromInt(int64(numberOfTrustlines)).Mul(baseReserveFee) // 1 base reserve per trustline
	minimumFundingAmount := baseReserves.Add(trustlineReserves)
	bootstrapAmount := decimal.NewFromInt(int64(s.nativeAssetBootstrapAmount))
	fundingAmount := decimal.Max(bootstrapAmount, minimumFundingAmount)

	log.Ctx(ctx).Infof("💰 Funding new account %q with %s XLMs", newAccount.Address, fundingAmount.StringFixed(2))

	return &txnbuild.CreateAccount{
		Destination: newAccount.Address,
		Amount:      fundingAmount.String(),
	}
}

func (s *DistributionAccountService) buildMergeAccountsOperations(ctx context.Context,
	newAccount, oldAccount schema.TransactionAccount,
	oldAccBalances map[data.Asset]decimal.Decimal,
) []txnbuild.Operation {
	var mergeOps []txnbuild.Operation

	for asset, balance := range oldAccBalances {
		if asset.IsNative() {
			continue
		}

		log.Ctx(ctx).Infof("🤝 Adding trustline for asset [%s:%s] to account %q", asset.Code, asset.Issuer, newAccount.Address)
		mergeOps = append(mergeOps,
			&txnbuild.ChangeTrust{
				Line: txnbuild.ChangeTrustAssetWrapper{
					Asset: asset.ToBasicAsset(),
				},
				Limit:         "", // Empty means no limit.
				SourceAccount: newAccount.Address,
			})

		if !balance.IsZero() {
			log.Ctx(ctx).Infof("💸 Transferring %s %s to account %q", balance.StringFixed(2), asset.Code, newAccount.Address)
			mergeOps = append(mergeOps, &txnbuild.Payment{
				Destination: newAccount.Address,
				Asset:       asset.ToBasicAsset(),
				Amount:      balance.String(),
			})
		}

		log.Ctx(ctx).Infof("🗑️ Removing trustline for asset [%s:%s] from account %q", asset.Code, asset.Issuer, oldAccount.Address)
		mergeOps = append(mergeOps,
			&txnbuild.ChangeTrust{
				Line: txnbuild.ChangeTrustAssetWrapper{
					Asset: asset.ToBasicAsset(),
				},
				Limit: "0", // Remove trustline by setting limit to 0
			})
	}

	// Add account merge as the final operation
	mergeOps = append(mergeOps, &txnbuild.AccountMerge{
		Destination: newAccount.Address,
	})

	return mergeOps
}

func (s *DistributionAccountService) buildAndSubmitTx(ctx context.Context, oldAccount, newAccount schema.TransactionAccount, ops []txnbuild.Operation) error {
	horizonClient := s.submitterEngine.HorizonClient
	hostAccount := s.submitterEngine.HostDistributionAccount()

	// 1. Refresh old account details for the latest sequence number
	oldAccountDetails, err := horizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: oldAccount.Address,
	})
	if err != nil {
		return fmt.Errorf("refreshing old account details: %w", err)
	}

	// 2. Build transaction from operations
	ledgerBounds, err := s.submitterEngine.LedgerNumberTracker.GetLedgerBounds()
	if err != nil {
		return fmt.Errorf("getting ledger bounds: %w", err)
	}
	preconditions := txnbuild.Preconditions{
		LedgerBounds: ledgerBounds,
		TimeBounds:   txnbuild.NewTimeout(15),
	}
	transferTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &oldAccountDetails,
			IncrementSequenceNum: true,
			Operations:           ops,
			BaseFee:              s.maxBaseFee,
			Preconditions:        preconditions,
		},
	)
	if err != nil {
		return fmt.Errorf("creating transaction for operations %w", err)
	}

	// 3. Sign Tx with the old and new accounts
	transferTx, err = s.submitterEngine.SignStellarTransaction(ctx, transferTx, oldAccount, newAccount)
	if err != nil {
		return fmt.Errorf("signing transfer transaction: %w", err)
	}

	// 4. Create fee bump transaction
	feeBumpTx, err := txnbuild.NewFeeBumpTransaction(
		txnbuild.FeeBumpTransactionParams{
			Inner:      transferTx,
			FeeAccount: hostAccount.Address,
			BaseFee:    s.maxBaseFee,
		},
	)
	if err != nil {
		return fmt.Errorf("creating fee bump transaction: %w", err)
	}

	// 5. Sign the fee bump with host account
	feeBumpTx, err = s.submitterEngine.SignFeeBumpStellarTransaction(ctx, feeBumpTx, hostAccount)
	if err != nil {
		return fmt.Errorf("signing fee bump transaction with host account %s: %w", hostAccount, err)
	}

	// 6. Submit the fee bumped transaction to the network
	_, err = horizonClient.SubmitFeeBumpTransactionWithOptions(
		feeBumpTx,
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	)
	if err != nil {
		return fmt.Errorf("submitting account migration transaction: %w", err)
	}

	log.Ctx(ctx).Infof("🎉 Successfully rotated from account %q to account %q", oldAccount.Address, newAccount.Address)

	return nil
}
