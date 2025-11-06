package cmd

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
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
}

// rotateDistributionAccount rotates the distribution account by creating a new account from the old account,
// funding the new account, adding trustlines to the new account, transferring assets to the new account,
// removing trustlines from the old account and merging the old account into the new account.
func (s *DistributionAccountService) rotateDistributionAccount(ctx context.Context) error {
	// 1. Get the current distribution account and check if it's a Stellar DB Vault account
	oldAccount, err := s.submitterEngine.DistributionAccountFromContext(ctx)
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

	// 4. Delete the old account
	if err = s.submitterEngine.Delete(ctx, oldAccount); err != nil {
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

	// 2. Get old account balances to create trustlines on the new account
	oldAccBalances, err := s.distAccService.GetBalances(ctx, &oldAccount)
	if err != nil {
		return nil, fmt.Errorf("getting old account balances: %w", err)
	}

	// 3. Build operations to create new account and merge old account
	var operations []txnbuild.Operation
	numberOfTrustlines := len(oldAccBalances) - 1 // Subtract 1 for the native asset
	operations = append(operations, s.buildCreateAccountOperation(ctx, newAccount, numberOfTrustlines))
	operations = append(operations, s.buildMergeAccountsOperations(ctx, newAccount, oldAccount, oldAccBalances)...)

	// 4. Build and submit the transaction.
	if err = s.buildAndSubmitTx(ctx, oldAccount, newAccount, operations); err != nil {
		return nil, fmt.Errorf("building and signing transaction: %w", err)
	}

	return &newAccount, nil
}

func (s *DistributionAccountService) buildCreateAccountOperation(ctx context.Context, newAccount schema.TransactionAccount, numberOfTrustlines int) txnbuild.Operation {
	baseReserveFee := decimal.RequireFromString("0.5")
	minimumFundingAmount := decimal.NewFromInt(2).Mul(baseReserveFee).Add(decimal.NewFromInt(int64(numberOfTrustlines)).Mul(baseReserveFee)) // 2 base reserves to exist on the ledger + 1 base reserve per trustline
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
