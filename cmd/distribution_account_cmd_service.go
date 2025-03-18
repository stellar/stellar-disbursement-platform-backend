package cmd

import (
	"context"
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	txSubSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

//go:generate mockery --name=DistAccCmdServiceInterface --case=underscore --structname=MockDistAccCmdServiceInterface
type DistAccCmdServiceInterface interface {
	RotateDistributionAccount(ctx context.Context) error
}

type DistAccCmdService struct {
	distAccService             services.DistributionAccountServiceInterface
	submitterEngine            engine.SubmitterEngine
	tenantManager              tenant.ManagerInterface
	maxBaseFee                 int64
	nativeAssetBootstrapAmount int
}

// RotateDistributionAccount rotates the distribution account by creating a new account from the old account,
// funding the new account, adding trustlines to the new account, transferring assets to the new account,
// removing trustlines from the old account and merging the old account into the new account.
func (s *DistAccCmdService) RotateDistributionAccount(ctx context.Context) error {
	// 1. Get the current distribution account and check if it's a Stellar DB Vault account
	oldAccount, err := s.submitterEngine.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		return fmt.Errorf("getting distribution account: %w", err)
	}
	if oldAccount.Type != schema.DistributionAccountStellarDBVault {
		return fmt.Errorf("distribution account rotation is only supported for Stellar DB Vault accounts")
	}

	// 2. Create a new Stellar account from the old account
	newAccount, err := s.createNewStellarAccountFromAccount(ctx, oldAccount)
	if err != nil {
		return fmt.Errorf("creating new account: %w", err)
	}

	// 3. Update the tenant with the new distribution account
	t, err := tenant.GetTenantFromContext(ctx)
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

	// 4. Delete the old account
	if err = s.submitterEngine.Delete(ctx, oldAccount); err != nil {
		return fmt.Errorf("deleting old account: %w", err)
	}

	return nil
}

func (s *DistAccCmdService) createNewStellarAccountFromAccount(ctx context.Context, oldAccount schema.TransactionAccount) (*schema.TransactionAccount, error) {
	// 1. Create new account and persist it
	log.Ctx(ctx).Infof("🔨 Creating new distribution account from account %s", truncAccount(oldAccount))
	newAccounts, err := s.submitterEngine.BatchInsert(ctx, schema.DistributionAccountStellarDBVault, 1)
	if err != nil {
		return nil, fmt.Errorf("inserting new account: %w", err)
	}
	if len(newAccounts) != 1 {
		return nil, fmt.Errorf("expected 1 new account, got %d", len(newAccounts))
	}
	newAccount := newAccounts[0]

	// 2. Fund the new account
	log.Ctx(ctx).Infof("💰 Funding new account %s with %d XLMs", truncAccount(newAccount), s.nativeAssetBootstrapAmount)
	hostDistributionAccPubKey := s.submitterEngine.HostDistributionAccount()
	err = txSubSvc.CreateAndFundAccount(ctx,
		s.submitterEngine,
		s.nativeAssetBootstrapAmount,
		hostDistributionAccPubKey.Address,
		newAccount.Address)
	if err != nil {
		return nil, fmt.Errorf("creating and funding account for key %s: %w", newAccount.Address, err)
	}

	// 3. Get old account balances to create trustlines on the new account
	oldAccBalances, err := s.distAccService.GetBalances(ctx, &oldAccount)
	if err != nil {
		return nil, fmt.Errorf("getting old account balances: %w", err)
	}

	// 4. Create trustlines on the new account
	if err := s.addTrustlinesToNewAccount(ctx, newAccount, oldAccBalances); err != nil {
		return nil, fmt.Errorf("adding trustlines to new account: %w", err)
	}

	// 5. Transfer assets to new account, remove trustlines and merge old account
	if err = s.mergeOldAccountIntoNewAccount(ctx, oldAccount, newAccount, oldAccBalances); err != nil {
		return nil, fmt.Errorf("merging old account into new account: %w", err)
	}

	return &newAccount, nil
}

func (s *DistAccCmdService) addTrustlinesToNewAccount(ctx context.Context,
	newAccount schema.TransactionAccount,
	oldAccBalances map[data.Asset]float64,
) error {
	var trustlineOps []txnbuild.Operation
	var assetsToTrust []string

	for asset := range oldAccBalances {
		if asset.IsNative() {
			continue
		}

		// Create trustline on new account
		trustlineOp := txnbuild.ChangeTrust{
			Line: txnbuild.ChangeTrustAssetWrapper{
				Asset: toBasicAsset(asset),
			},
			Limit:         "", // empty means no limit.
			SourceAccount: newAccount.Address,
		}
		trustlineOps = append(trustlineOps, &trustlineOp)
		assetsToTrust = append(assetsToTrust, asset.Code)
	}

	// Submit trustline creation transaction if there are any trustlines
	horizonClient := s.submitterEngine.HorizonClient
	if len(trustlineOps) > 0 {
		// Get the new account details for sequence number
		newAccountDetails, err := horizonClient.AccountDetail(horizonclient.AccountRequest{
			AccountID: newAccount.Address,
		})
		if err != nil {
			return fmt.Errorf("getting new account details: %w", err)
		}

		trustlineTx, err := s.buildTransactionForOperations(newAccountDetails, trustlineOps)
		if err != nil {
			return fmt.Errorf("building transaction for trustline operations: %w", err)
		}

		// Sign and submit
		trustlineTx, err = s.submitterEngine.SignStellarTransaction(ctx, trustlineTx, newAccount)
		if err != nil {
			return fmt.Errorf("signing trustline transaction: %w", err)
		}

		log.Ctx(ctx).Infof("🤝 Adding trustline(s) %v to account %s", assetsToTrust, truncAccount(newAccount))
		_, err = horizonClient.SubmitTransactionWithOptions(trustlineTx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
		if err != nil {
			return fmt.Errorf("submitting trustline transaction: %w", err)
		}
	}

	return nil
}

// mergeOldAccountIntoNewAccount transfers assets from the old account to the new account,
// removes trustlines from the old account and merges the old account into the new account.
func (s *DistAccCmdService) mergeOldAccountIntoNewAccount(ctx context.Context,
	oldAccount schema.TransactionAccount,
	newAccount schema.TransactionAccount,
	oldAccBalances map[data.Asset]float64,
) error {
	// Build merge operations
	mergeOps := buildMergeOperations(ctx, oldAccount, oldAccBalances, newAccount)

	// Refresh old account details for the latest sequence number
	horizonClient := s.submitterEngine.HorizonClient
	oldAccountDetails, err := horizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: oldAccount.Address,
	})
	if err != nil {
		return fmt.Errorf("refreshing old account details: %w", err)
	}

	// Build transfer and merge transaction
	transferTx, err := s.buildTransactionForOperations(oldAccountDetails, mergeOps)
	if err != nil {
		return fmt.Errorf("building transaction for merge operations: %w", err)
	}

	// Sign with old account
	transferTx, err = s.submitterEngine.SignStellarTransaction(ctx, transferTx, oldAccount)
	if err != nil {
		return fmt.Errorf("signing transfer transaction: %w", err)
	}

	// Create fee bump transaction
	hostAccount := s.submitterEngine.HostDistributionAccount()

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

	// Sign the fee bump
	feeBumpTx, err = s.submitterEngine.SignFeeBumpStellarTransaction(ctx, feeBumpTx, hostAccount)
	if err != nil {
		return fmt.Errorf("signing fee bump transaction: %w", err)
	}

	// Submit the fee bumped transaction
	_, err = horizonClient.SubmitFeeBumpTransactionWithOptions(
		feeBumpTx,
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	)
	if err != nil {
		return fmt.Errorf("submitting account migration transaction: %w", err)
	}

	log.Ctx(ctx).Infof("🎉 Successfully rotated from account %s to %s", truncAccount(oldAccount), truncAccount(newAccount))
	return nil
}

func (s *DistAccCmdService) buildTransactionForOperations(accountDetails horizon.Account, ops []txnbuild.Operation) (*txnbuild.Transaction, error) {
	ledgerBounds, err := s.submitterEngine.LedgerNumberTracker.GetLedgerBounds()
	if err != nil {
		return nil, fmt.Errorf("getting ledger bounds: %w", err)
	}
	preconditions := txnbuild.Preconditions{
		LedgerBounds: ledgerBounds,
		TimeBounds:   txnbuild.NewTimeout(15),
	}
	transferTx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: accountDetails.AccountID,
				Sequence:  accountDetails.Sequence,
			},
			IncrementSequenceNum: true,
			Operations:           ops,
			BaseFee:              s.maxBaseFee,
			Preconditions:        preconditions,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("creating transaction for operations %w", err)
	}
	return transferTx, nil
}

// buildMergeOperations creates a list of operation
// s to transfer assets from the old account to the new account,
// remove trustlines from the old account and merge the old account into the new account.
func buildMergeOperations(ctx context.Context,
	oldAccount schema.TransactionAccount,
	oldAccBalances map[data.Asset]float64,
	newAccount schema.TransactionAccount,
) []txnbuild.Operation {
	var mergeOps []txnbuild.Operation
	for asset, balance := range oldAccBalances {
		// Skip native XLM balance as it will be transferred during merge
		if asset.IsNative() {
			continue
		}

		// If there's a balance on this asset, create a payment operation
		if balance != 0 {
			paymentOp := txnbuild.Payment{
				Destination:   newAccount.Address,
				Asset:         toBasicAsset(asset),
				Amount:        utils.FloatToString(balance),
				SourceAccount: oldAccount.Address,
			}
			mergeOps = append(mergeOps, &paymentOp)
			log.Ctx(ctx).Infof("💸 Transferring %.2f %s to account %s", balance, asset.Code, truncAccount(newAccount))
		}

		// Remove trustline from old account
		removeTrustlineOp := txnbuild.ChangeTrust{
			Line: txnbuild.ChangeTrustAssetWrapper{
				Asset: toBasicAsset(asset),
			},
			Limit:         "0", // Setting limit to 0 removes the trustline
			SourceAccount: oldAccount.Address,
		}
		mergeOps = append(mergeOps, &removeTrustlineOp)
		log.Ctx(ctx).Infof("🗑️ Removing trustline for asset %s from account %s", asset.Code, truncAccount(oldAccount))
	}

	// Add account merge as the final operation
	mergeOp := txnbuild.AccountMerge{
		Destination:   newAccount.Address,
		SourceAccount: oldAccount.Address,
	}
	mergeOps = append(mergeOps, &mergeOp)

	return mergeOps
}

func toBasicAsset(a data.Asset) txnbuild.Asset {
	if a.IsNative() {
		return txnbuild.NativeAsset{}
	}
	return txnbuild.CreditAsset{Code: a.Code, Issuer: a.Issuer}
}

func truncAccount(acc schema.TransactionAccount) string {
	return utils.TruncateString(acc.Address, 4)
}

var _ DistAccCmdServiceInterface = (*DistAccCmdService)(nil)
