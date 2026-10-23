package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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

	// Every tenant has a default distribution wallet; multi-wallet eligibility is
	// derived from its account type. Seed a DB_VAULT default so the eligible path runs.
	defaultAddr := keypair.MustRandom().Address()
	_, err = models.DistributionWallets.EnsureDefaultWallet(ctx, dbConnectionPool, &defaultAddr, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive)
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

// Mixed-custody guard: v1 restricts multi-wallet to DB_VAULT tenants. A tenant whose
// default distribution account is STELLAR.ENV or CIRCLE must not be able to provision a
// (DB_VAULT) additional wallet — that would create a mixed-custody tenant and a later
// promote-to-default would silently flip the tenant's account type + balance source.
func Test_DistributionWalletManagementService_CreateWallet_eligibility(t *testing.T) {
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
	hostAccount := schema.NewDefaultHostAccount(keypair.MustRandom().Address())

	newSvc := func(t *testing.T) *DistributionWalletManagementService {
		t.Helper()
		sigService, _, distAccResolver := signing.NewMockSignatureService(t)
		distAccResolver.On("HostDistributionAccount").Return(hostAccount).Maybe()
		svc, sErr := NewDistributionWalletManagementService(models, engine.SubmitterEngine{
			HorizonClient:       &horizonclient.MockClient{},
			SignatureService:    sigService,
			LedgerNumberTracker: preconditionsMocks.NewMockLedgerNumberTracker(t),
			MaxBaseFee:          100 * txnbuild.MinBaseFee,
		}, walletKeyService, 5)
		require.NoError(t, sErr)
		return svc
	}

	for _, ineligible := range []schema.AccountType{
		schema.DistributionAccountStellarEnv,
		schema.DistributionAccountCircleDBVault,
	} {
		t.Run(fmt.Sprintf("rejects multi-wallet for %s tenants", ineligible), func(t *testing.T) {
			addr := keypair.MustRandom().Address()
			_, eErr := models.DistributionWallets.EnsureDefaultWallet(ctx, dbConnectionPool, &addr, ineligible, schema.AccountStatusActive)
			require.NoError(t, eErr)

			svc := newSvc(t)
			_, cErr := svc.CreateWallet(ctx, data.DistributionWalletInsert{Name: "program-x"})
			require.ErrorIs(t, cErr, ErrTenantNotEligibleForMultiWallet)

			// Guard fails before any row/key is created — only the default remains.
			count, cntErr := models.DistributionWallets.Count(ctx, dbConnectionPool)
			require.NoError(t, cntErr)
			assert.Equal(t, 1, count, "no wallet row may be created past the eligibility guard")
		})
	}
}

// Service-level coverage for the ArchiveWallet guards (previously only exercised through a
// mock at the handler layer): the default wallet is protected, and unknown ids surface a
// not-found error rather than a generic failure.
func Test_DistributionWalletManagementService_ArchiveWallet_guards(t *testing.T) {
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

	// Archive guards are DB-only (no on-chain work), so a minimal engine is sufficient.
	svc, err := NewDistributionWalletManagementService(models, engine.SubmitterEngine{}, walletKeyService, 5)
	require.NoError(t, err)

	addr := keypair.MustRandom().Address()
	def, err := models.DistributionWallets.EnsureDefaultWallet(ctx, dbConnectionPool, &addr, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive)
	require.NoError(t, err)

	t.Run("the default wallet cannot be archived", func(t *testing.T) {
		_, aErr := svc.ArchiveWallet(ctx, def.ID)
		require.ErrorIs(t, aErr, ErrCannotArchiveDefaultWallet)
	})

	t.Run("archiving an unknown wallet returns not-found", func(t *testing.T) {
		_, aErr := svc.ArchiveWallet(ctx, "00000000-0000-0000-0000-000000000000")
		require.ErrorIs(t, aErr, data.ErrRecordNotFound)
	})
}

// Happy-path service coverage for archive + promote (previously only the error mapping was
// tested, via a mock at the handler layer). A model-inserted wallet is ACTIVE by default,
// so it can be archived/promoted directly without on-chain provisioning.
func Test_DistributionWalletManagementService_ArchiveAndPromote(t *testing.T) {
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
	svc, err := NewDistributionWalletManagementService(models, engine.SubmitterEngine{}, walletKeyService, 5)
	require.NoError(t, err)

	addr := keypair.MustRandom().Address()
	_, err = models.DistributionWallets.EnsureDefaultWallet(ctx, dbConnectionPool, &addr, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive)
	require.NoError(t, err)

	t.Run("archives a non-default active wallet", func(t *testing.T) {
		w, iErr := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
			Name: "to-archive", AccountType: schema.DistributionAccountStellarDBVault,
		})
		require.NoError(t, iErr)
		w, iErr = models.DistributionWallets.Activate(ctx, dbConnectionPool, w.ID)
		require.NoError(t, iErr)

		archived, aErr := svc.ArchiveWallet(ctx, w.ID)
		require.NoError(t, aErr)
		assert.Equal(t, data.ArchivedDistributionWalletStatus, archived.Status)
		require.NotNil(t, archived.ArchivedAt)
	})

	t.Run("promotes a non-default active wallet to default", func(t *testing.T) {
		w, iErr := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
			Name: "to-promote", AccountType: schema.DistributionAccountStellarDBVault,
		})
		require.NoError(t, iErr)
		w, iErr = models.DistributionWallets.Activate(ctx, dbConnectionPool, w.ID)
		require.NoError(t, iErr)

		promoted, pErr := svc.PromoteToDefault(ctx, w.ID, nil)
		require.NoError(t, pErr)
		assert.True(t, promoted.IsDefault)

		current, gErr := models.DistributionWallets.GetDefault(ctx, dbConnectionPool)
		require.NoError(t, gErr)
		assert.Equal(t, w.ID, current.ID, "the promoted wallet is now the sole default")
	})
}

// addTrustlines derives the new account's trustlines from the enabled wallet providers' supported
// assets, retired (soft-deleted) ones included: the provider link is tenant intent, and once this
// account trusts the asset it is back in use, so it returns to the catalogue. Only after the
// ChangeTrust succeeded — trustlines are best-effort, and an account that trusts nothing must not
// revive anything.
func Test_DistributionWalletManagementService_addTrustlines_restoresRetiredAssets(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	provider := data.CreateWalletFixture(t, ctx, dbConnectionPool, "provider", "https://provider.test", "provider.test", "provider://")
	live := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDT", "GBHC5ADV2XYITXCYC5F6X6BM2OYTYHV4ZU2JF6QWJORJQE2O7RKH2LAQ")
	retired := data.CreateAssetFixture(t, ctx, dbConnectionPool, "EURC", "GB3Q6QDZYTHWT7E5PVS3W7FUT5GVAFC5KSZFFLPU25GO7VTC3NM2ZTVO")
	data.AssociateAssetWithWalletFixture(t, ctx, dbConnectionPool, live.ID, provider.ID)
	data.AssociateAssetWithWalletFixture(t, ctx, dbConnectionPool, retired.ID, provider.ID)

	retire := func(t *testing.T) {
		t.Helper()
		_, sdErr := models.Assets.SoftDelete(ctx, dbConnectionPool, retired.ID)
		require.NoError(t, sdErr)
	}
	deletedAt := func(t *testing.T, id string) *time.Time {
		t.Helper()
		a, gErr := models.Assets.Get(ctx, id)
		require.NoError(t, gErr)
		return a.DeletedAt
	}

	kp := keypair.MustRandom()
	address := kp.Address()
	wallet := data.DistributionWallet{
		ID:            "new-wallet",
		Address:       &address,
		AccountType:   schema.DistributionAccountStellarDBVault,
		AccountStatus: schema.AccountStatusActive,
	}
	account := schema.TransactionAccount{Address: address, Type: wallet.AccountType, Status: wallet.AccountStatus}

	newService := func(t *testing.T, mHorizonClient *horizonclient.MockClient, submitted **txnbuild.Transaction) *DistributionWalletManagementService {
		t.Helper()

		signatureService, sigRouter, _ := signing.NewMockSignatureService(t)
		sigRouter.
			On("SignStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.Transaction"), account).
			Run(func(args mock.Arguments) { *submitted = args.Get(1).(*txnbuild.Transaction) }).
			Return(&txnbuild.Transaction{}, nil).Once()

		return &DistributionWalletManagementService{
			Models: models,
			SubmitterEngine: engine.SubmitterEngine{
				SignatureService:    signatureService,
				HorizonClient:       mHorizonClient,
				LedgerNumberTracker: preconditionsMocks.NewMockLedgerNumberTracker(t),
				MaxBaseFee:          100,
			},
		}
	}

	t.Run("trusts the retired asset too and restores it to the catalogue", func(t *testing.T) {
		retire(t)
		require.NotNil(t, deletedAt(t, retired.ID))

		mHorizonClient := &horizonclient.MockClient{}
		defer mHorizonClient.AssertExpectations(t)
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: address}).
			Return(horizon.Account{AccountID: address, Sequence: 1}, nil).Once()
		mHorizonClient.
			On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, nil).Once()

		var submitted *txnbuild.Transaction
		svc := newService(t, mHorizonClient, &submitted)
		require.NoError(t, svc.addTrustlines(ctx, wallet))

		require.NotNil(t, submitted)
		codes := make([]string, 0, 2)
		for _, op := range submitted.Operations() {
			changeTrust, ok := op.(*txnbuild.ChangeTrust)
			require.True(t, ok)
			codes = append(codes, changeTrust.Line.GetCode())
		}
		assert.ElementsMatch(t, []string{live.Code, retired.Code}, codes)

		assert.Nil(t, deletedAt(t, retired.ID), "the retired asset is back in use")
		assert.Nil(t, deletedAt(t, live.ID))
	})

	t.Run("a failed ChangeTrust leaves the retired asset retired", func(t *testing.T) {
		retire(t)

		mHorizonClient := &horizonclient.MockClient{}
		defer mHorizonClient.AssertExpectations(t)
		mHorizonClient.
			On("AccountDetail", horizonclient.AccountRequest{AccountID: address}).
			Return(horizon.Account{AccountID: address, Sequence: 1}, nil).Once()
		mHorizonClient.
			On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
			Return(horizon.Transaction{}, errors.New("horizon exploded")).Once()

		var submitted *txnbuild.Transaction
		svc := newService(t, mHorizonClient, &submitted)
		require.Error(t, svc.addTrustlines(ctx, wallet))

		assert.NotNil(t, deletedAt(t, retired.ID), "nothing trusts it, so it stays retired")
	})
}
