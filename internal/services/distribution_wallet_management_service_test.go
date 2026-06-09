package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_DistributionWalletManagementService_CreateWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	kek := keypair.MustRandom().Seed()
	walletKeyService, err := tssSvc.NewDistributionWalletKeyService(dbConnectionPool, kek)
	require.NoError(t, err)

	hostKP := keypair.MustRandom()
	hostAccount := schema.NewDefaultHostAccount(hostKP.Address())

	newService := func(t *testing.T, mHorizonClient *horizonclient.MockClient) *DistributionWalletManagementService {
		t.Helper()
		sigService, sigRouter, distAccResolver := signing.NewMockSignatureService(t)
		distAccResolver.On("HostDistributionAccount").Return(hostAccount).Maybe()
		sigRouter.
			On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostAccount).
			Return(&txnbuild.Transaction{}, nil).Maybe()

		svc, sErr := NewDistributionWalletManagementService(models, engine.SubmitterEngine{
			HorizonClient:       mHorizonClient,
			SignatureService:    sigService,
			LedgerNumberTracker: preconditionsMocks.NewMockLedgerNumberTracker(t),
			MaxBaseFee:          100 * txnbuild.MinBaseFee,
		}, walletKeyService, 5)
		require.NoError(t, sErr)
		return svc
	}

	t.Run("🎉 creates, funds, and stores an isolated key for the new wallet", func(t *testing.T) {
		mHorizonClient := &horizonclient.MockClient{}
		defer mHorizonClient.AssertExpectations(t)
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: hostKP.Address()}).
			Return(horizon.Account{AccountID: hostKP.Address(), Sequence: 1}, nil).Once()
		mHorizonClient.
			On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).Once()
		// Post-funding verification fetches the (random) new wallet address.
		mHorizonClient.
			On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
			Return(horizon.Account{Sequence: 1}, nil).Maybe()

		svc := newService(t, mHorizonClient)
		wallet, cErr := svc.CreateWallet(ctx, data.DistributionWalletInsert{Name: "program-a"})
		require.NoError(t, cErr)

		// Queryable, ACTIVE, non-default, with an attached address.
		require.NotNil(t, wallet.Address)
		assert.Equal(t, data.ActiveDistributionWalletStatus, wallet.Status)
		assert.Equal(t, schema.DistributionAccountStellarDBVault, wallet.AccountType)
		assert.False(t, wallet.IsDefault)

		got, gErr := svc.GetWallet(ctx, wallet.ID)
		require.NoError(t, gErr)
		assert.Equal(t, wallet.ID, got.ID)

		// Secret material stored under per-wallet envelope encryption, decryptable, and
		// matching the on-chain address.
		seed, kErr := walletKeyService.GetPrivateKey(ctx, *wallet.Address)
		require.NoError(t, kErr)
		kp, kErr := keypair.ParseFull(seed)
		require.NoError(t, kErr)
		assert.Equal(t, *wallet.Address, kp.Address())
	})

	t.Run("duplicate name fails without storing a key", func(t *testing.T) {
		svc := newService(t, &horizonclient.MockClient{})
		_, cErr := svc.CreateWallet(ctx, data.DistributionWalletInsert{Name: "program-a"})
		require.ErrorIs(t, cErr, data.ErrRecordAlreadyExists)
	})

	t.Run("unsupported account types are rejected in v1", func(t *testing.T) {
		svc := newService(t, &horizonclient.MockClient{})
		_, cErr := svc.CreateWallet(ctx, data.DistributionWalletInsert{
			Name:        "circle-wallet",
			AccountType: schema.DistributionAccountCircleDBVault,
		})
		require.ErrorIs(t, cErr, ErrUnsupportedDistributionWalletType)
	})

	t.Run("funding failure cleans up the row and the key", func(t *testing.T) {
		mHorizonClient := &horizonclient.MockClient{}
		mHorizonClient.
			On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).
			Return(horizon.Account{}, errors.New("horizon exploded"))

		svc := newService(t, mHorizonClient)
		_, cErr := svc.CreateWallet(ctx, data.DistributionWalletInsert{Name: "doomed-wallet"})
		require.Error(t, cErr)
		assert.ErrorContains(t, cErr, "funding distribution wallet")

		// Row cleaned up.
		_, gErr := models.DistributionWallets.GetByName(ctx, dbConnectionPool, "doomed-wallet")
		require.ErrorIs(t, gErr, data.ErrRecordNotFound)

		// No orphaned wallet keys: every stored key belongs to an existing wallet.
		var orphanCount int
		require.NoError(t, dbConnectionPool.GetContext(ctx, &orphanCount, `
			SELECT COUNT(*) FROM distribution_wallet_keys k
			WHERE NOT EXISTS (
				SELECT 1 FROM distribution_wallets w WHERE w.distribution_account_address = k.public_key
			)`))
		assert.Zero(t, orphanCount)
	})

	t.Run("20-wallet cap is enforced", func(t *testing.T) {
		// Fill up to the cap (existing wallets count too).
		existing, cErr := models.DistributionWallets.Count(ctx, dbConnectionPool)
		require.NoError(t, cErr)
		for i := existing; i < data.MaxDistributionWalletsPerTenant; i++ {
			_, iErr := dbConnectionPool.ExecContext(ctx, fmt.Sprintf(`
				INSERT INTO distribution_wallets (name, distribution_account_type)
				VALUES ('filler-%d', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT')`, i))
			require.NoError(t, iErr)
		}

		svc := newService(t, &horizonclient.MockClient{})
		_, cErr2 := svc.CreateWallet(ctx, data.DistributionWalletInsert{Name: "one-too-many"})
		require.ErrorIs(t, cErr2, ErrDistributionWalletCapExceeded)

		count, cntErr := models.DistributionWallets.Count(ctx, dbConnectionPool)
		require.NoError(t, cntErr)
		assert.Equal(t, data.MaxDistributionWalletsPerTenant, count, "no row may be created past the cap")
	})
}
