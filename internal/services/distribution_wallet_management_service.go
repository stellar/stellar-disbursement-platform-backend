package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

var (
	// ErrDistributionWalletCapExceeded is returned when a tenant attempts to create more than
	// the v1 per-tenant wallet cap.
	ErrDistributionWalletCapExceeded = fmt.Errorf("tenant has reached the maximum of %d distribution wallets", data.MaxDistributionWalletsPerTenant)
	// ErrUnsupportedDistributionWalletType is returned for account types that cannot be
	// provisioned per-wallet in v1.
	ErrUnsupportedDistributionWalletType = errors.New("only DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT wallets can be created in v1")
)

// DistributionWalletManagementServiceInterface manages the lifecycle of a tenant's
// distribution wallets (the sending accounts), Owner-only at the API layer.
type DistributionWalletManagementServiceInterface interface {
	CreateWallet(ctx context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error)
	GetWallet(ctx context.Context, id string) (*data.DistributionWallet, error)
	ListWallets(ctx context.Context, includeArchived bool) ([]data.DistributionWallet, error)
}

type DistributionWalletManagementService struct {
	Models                     *data.Models
	SubmitterEngine            engine.SubmitterEngine
	WalletKeyService           tssSvc.DistributionWalletKeyServiceInterface
	NativeAssetBootstrapAmount int
}

var _ DistributionWalletManagementServiceInterface = (*DistributionWalletManagementService)(nil)

func NewDistributionWalletManagementService(
	models *data.Models,
	submitterEngine engine.SubmitterEngine,
	walletKeyService tssSvc.DistributionWalletKeyServiceInterface,
	nativeAssetBootstrapAmount int,
) (*DistributionWalletManagementService, error) {
	if models == nil {
		return nil, fmt.Errorf("models cannot be nil")
	}
	if walletKeyService == nil {
		return nil, fmt.Errorf("walletKeyService cannot be nil")
	}

	return &DistributionWalletManagementService{
		Models:                     models,
		SubmitterEngine:            submitterEngine,
		WalletKeyService:           walletKeyService,
		NativeAssetBootstrapAmount: nativeAssetBootstrapAmount,
	}, nil
}

// CreateWallet provisions a new, independently funded distribution wallet for the tenant:
// it reserves the wallet row, generates a fresh keypair stored with per-wallet envelope
// encryption, funds the account from the host distribution account, and adds trustlines for
// the tenant's enabled assets. On provisioning failure the never-used row and key are cleaned
// up — archive-don't-delete only protects wallets that have signed transactions.
func (s *DistributionWalletManagementService) CreateWallet(ctx context.Context, insert data.DistributionWalletInsert) (*data.DistributionWallet, error) {
	if insert.AccountType == "" {
		insert.AccountType = schema.DistributionAccountStellarDBVault
	}
	if insert.AccountType != schema.DistributionAccountStellarDBVault {
		return nil, fmt.Errorf("creating wallet with account type %q: %w", insert.AccountType, ErrUnsupportedDistributionWalletType)
	}

	dbPool := s.Models.DBConnectionPool

	count, err := s.Models.DistributionWallets.Count(ctx, dbPool)
	if err != nil {
		return nil, fmt.Errorf("counting distribution wallets: %w", err)
	}
	if count >= data.MaxDistributionWalletsPerTenant {
		return nil, ErrDistributionWalletCapExceeded
	}

	wallet, err := s.Models.DistributionWallets.Insert(ctx, dbPool, insert)
	if err != nil {
		return nil, fmt.Errorf("inserting distribution wallet: %w", err)
	}

	kp, err := keypair.Random()
	if err != nil {
		s.cleanupWallet(ctx, wallet.ID, "")
		return nil, fmt.Errorf("generating keypair for distribution wallet %q: %w", wallet.ID, err)
	}

	if err = s.WalletKeyService.StoreKey(ctx, kp.Address(), kp.Seed()); err != nil {
		s.cleanupWallet(ctx, wallet.ID, "")
		return nil, fmt.Errorf("storing key for distribution wallet %q: %w", wallet.ID, err)
	}

	updatedWallet, err := s.Models.DistributionWallets.UpdateAddress(ctx, dbPool, wallet.ID, kp.Address())
	if err != nil {
		s.cleanupWallet(ctx, wallet.ID, kp.Address())
		return nil, fmt.Errorf("attaching address to distribution wallet %q: %w", wallet.ID, err)
	}
	wallet = updatedWallet

	hostAccount := s.SubmitterEngine.HostDistributionAccount()
	if err = tssSvc.CreateAndFundAccount(ctx, s.SubmitterEngine, s.NativeAssetBootstrapAmount, hostAccount.Address, kp.Address()); err != nil {
		s.cleanupWallet(ctx, wallet.ID, kp.Address())
		return nil, fmt.Errorf("funding distribution wallet %q: %w", wallet.ID, err)
	}

	// Trustlines are best-effort: the wallet is already live and funded on-chain, so a
	// trustline failure must not orphan it. Missing trustlines only block non-native assets
	// and can be re-attempted by operations.
	if err = s.addTrustlines(ctx, *wallet); err != nil {
		log.Ctx(ctx).Errorf("adding trustlines for new distribution wallet %q (%s): %v — wallet is funded and ACTIVE; trustlines must be added manually", wallet.ID, kp.Address(), err)
	}

	return wallet, nil
}

// GetWallet returns one distribution wallet by id.
func (s *DistributionWalletManagementService) GetWallet(ctx context.Context, id string) (*data.DistributionWallet, error) {
	wallet, err := s.Models.DistributionWallets.Get(ctx, s.Models.DBConnectionPool, id)
	if err != nil {
		return nil, fmt.Errorf("getting distribution wallet %q: %w", id, err)
	}
	return wallet, nil
}

// ListWallets returns the tenant's distribution wallets, optionally including archived ones
// (operational surfaces hide archived wallets; audit/admin views include them).
func (s *DistributionWalletManagementService) ListWallets(ctx context.Context, includeArchived bool) ([]data.DistributionWallet, error) {
	wallets, err := s.Models.DistributionWallets.GetAll(ctx, s.Models.DBConnectionPool, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("listing distribution wallets: %w", err)
	}
	return wallets, nil
}

// cleanupWallet removes a freshly created wallet row (and its stored key, when one exists)
// after a provisioning failure. Best-effort: cleanup failures are logged, not returned, so the
// original provisioning error stays primary.
func (s *DistributionWalletManagementService) cleanupWallet(ctx context.Context, walletID, publicKey string) {
	if publicKey != "" {
		if err := s.WalletKeyService.DeleteKey(ctx, publicKey); err != nil {
			log.Ctx(ctx).Errorf("cleaning up key for failed distribution wallet %q (%s): %v", walletID, publicKey, err)
		}
	}
	if err := s.Models.DistributionWallets.Delete(ctx, s.Models.DBConnectionPool, walletID); err != nil {
		log.Ctx(ctx).Errorf("cleaning up failed distribution wallet %q: %v", walletID, err)
	}
}

// addTrustlines adds trustlines on the new wallet for all non-native assets supported by the
// tenant's enabled recipient wallet providers, mirroring tenant-provisioning behavior.
func (s *DistributionWalletManagementService) addTrustlines(ctx context.Context, wallet data.DistributionWallet) error {
	wallets, err := s.Models.Wallets.FindWallets(ctx, data.NewFilter(data.FilterEnabledWallets, true))
	if err != nil {
		return fmt.Errorf("listing enabled wallets: %w", err)
	}

	supportedAssets := make(map[string]data.Asset)
	for _, w := range wallets {
		for _, asset := range w.Assets {
			if asset.IsNative() {
				continue
			}
			supportedAssets[fmt.Sprintf("%s:%s", asset.Code, asset.Issuer)] = asset
		}
	}
	if len(supportedAssets) == 0 {
		return nil
	}

	assetsToTrust := make([]data.Asset, 0, len(supportedAssets))
	for _, asset := range supportedAssets {
		assetsToTrust = append(assetsToTrust, asset)
	}

	distAccount := schema.TransactionAccount{
		Address: *wallet.Address,
		Type:    wallet.AccountType,
		Status:  wallet.AccountStatus,
	}
	if _, err := tssSvc.AddTrustlines(ctx, s.SubmitterEngine, distAccount, assetsToTrust); err != nil {
		return fmt.Errorf("submitting change trust transaction: %w", err)
	}

	return nil
}
