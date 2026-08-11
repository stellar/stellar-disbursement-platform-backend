package cmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	tssStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_DistAccCmdService_RotateDistributionAccount(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Create test data
	oldAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
	newAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
	hostAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())

	testCases := []struct {
		name       string
		setupMocks func(
			distAccServiceMock *mocks.MockDistributionAccountService,
			horizonClientMock *horizonclient.MockClient,
			signerRouterMock *sigMocks.MockSignerRouter,
			distAccountResolverMock *sigMocks.MockDistributionAccountResolver,
		)
		expectedError string
	}{
		{
			name: "🎉 successfully rotates distribution account",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
				distAccountResolverMock *sigMocks.MockDistributionAccountResolver,
			) {
				distAccountResolverMock.
					On("DistributionAccountFromContext", mock.Anything).
					Return(oldAccount, nil).
					Once()

				setupAccountCreationMocks(distAccServiceMock, horizonClientMock, signerRouterMock, oldAccount, newAccount)

				signerRouterMock.
					On("Delete", mock.Anything, oldAccount).
					Return(nil).
					Once()
			},
			expectedError: "",
		},
		{
			name: "fails when distribution account resolver returns error",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
				distAccountResolverMock *sigMocks.MockDistributionAccountResolver,
			) {
				distAccountResolverMock.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{}, fmt.Errorf("account resolver error")).
					Once()
			},
			expectedError: "getting distribution account: account resolver error",
		},
		{
			name: "fails when distribution account is not a Stellar DB Vault account",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
				distAccountResolverMock *sigMocks.MockDistributionAccountResolver,
			) {
				nonStellarDBVaultAccount := oldAccount
				nonStellarDBVaultAccount.Type = schema.DistributionAccountStellarEnv

				distAccountResolverMock.
					On("DistributionAccountFromContext", mock.Anything).
					Return(nonStellarDBVaultAccount, nil).
					Once()
			},
			expectedError: "distribution account rotation is only supported for Stellar DB Vault accounts",
		},
		{
			name: "fails when createNewStellarAccountFromAccount returns error",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
				distAccountResolverMock *sigMocks.MockDistributionAccountResolver,
			) {
				distAccountResolverMock.
					On("DistributionAccountFromContext", mock.Anything).
					Return(oldAccount, nil).
					Once()

				signerRouterMock.
					On("BatchInsert", mock.Anything, schema.DistributionAccountStellarDBVault, 1).
					Return(nil, fmt.Errorf("batch insert error")).
					Once()
			},
			expectedError: "creating new account: inserting new account: batch insert error",
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks
			distAccServiceMock := mocks.NewMockDistributionAccountService(t)
			ledgerTrackerMock := preconditionsMocks.NewMockLedgerNumberTracker(t)
			horizonClientMock := &horizonclient.MockClient{}
			signatureServiceMock, signerRouterMock, distAccResolverMock := signing.NewMockSignatureService(t)

			ledgerTrackerMock.
				On("GetLedgerBounds").
				Return(&txnbuild.LedgerBounds{MinLedger: 100, MaxLedger: 200}, nil).
				Maybe()
			distAccResolverMock.On("HostDistributionAccount").
				Return(hostAccount).
				Maybe()

			submitterEngine := engine.SubmitterEngine{
				HorizonClient:       horizonClientMock,
				LedgerNumberTracker: ledgerTrackerMock,
				SignatureService:    signatureServiceMock,
				MaxBaseFee:          150,
			}

			tc.setupMocks(
				distAccServiceMock,
				horizonClientMock,
				signerRouterMock,
				distAccResolverMock,
			)

			// Create the service under test
			tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
			testTenant := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, fmt.Sprintf("tenant-%d", i), oldAccount.Address)
			ctx = sdpcontext.SetTenantInContext(ctx, testTenant)

			service := DistributionAccountService{
				distAccService:             distAccServiceMock,
				submitterEngine:            submitterEngine,
				tenantManager:              tenantManager,
				maxBaseFee:                 100,
				nativeAssetBootstrapAmount: 5,
				models:                     models,
			}

			// Call the method
			distAccCmdService := DistAccCmdService{}
			rotateErr := distAccCmdService.RotateDistributionAccount(ctx, service)

			// Assert expectations
			if tc.expectedError != "" {
				require.Error(t, rotateErr)
				assert.Contains(t, rotateErr.Error(), tc.expectedError)
				updatedTenant, tntErr := tenantManager.GetTenantByID(ctx, testTenant.ID)
				require.NoError(t, tntErr)
				assert.Equal(t, oldAccount.Address, *updatedTenant.DistributionAccountAddress)
			} else {
				require.NoError(t, rotateErr)
				updatedTenant, tntErr := tenantManager.GetTenantByID(ctx, testTenant.ID)
				require.NoError(t, tntErr)
				assert.Equal(t, newAccount.Address, *updatedTenant.DistributionAccountAddress)

				// The default distribution wallet row must move with the tenant: leaving it on
				// the merged-away account is the bug this rotation path used to have.
				defaultWallet, dwErr := models.DistributionWallets.GetDefault(ctx, dbConnectionPool)
				require.NoError(t, dwErr)
				require.NotNil(t, defaultWallet.Address)
				assert.Equal(t, newAccount.Address, *defaultWallet.Address)
			}

			horizonClientMock.AssertExpectations(t)
		})
	}
}

func setupAccountCreationMocks(
	distAccServiceMock *mocks.MockDistributionAccountService,
	horizonClientMock *horizonclient.MockClient,
	signerRouterMock *sigMocks.MockSignerRouter,
	oldAccount, newAccount schema.TransactionAccount,
) {
	assetUSDC := data.Asset{
		Code:   assets.USDCAssetCode,
		Issuer: assets.USDCAssetIssuerTestnet,
	}

	signerRouterMock.
		On("BatchInsert", mock.Anything, schema.DistributionAccountStellarDBVault, 1).
		Return([]schema.TransactionAccount{newAccount}, nil).
		Once()

	balances := map[data.Asset]decimal.Decimal{
		assetUSDC: decimal.NewFromFloat(10.0),
	}

	distAccServiceMock.
		On("GetBalances", mock.Anything, &oldAccount).
		Return(balances, nil).
		Once()

	horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
		AccountID: oldAccount.Address,
	}).Return(horizon.Account{
		AccountID: oldAccount.Address,
		Sequence:  100,
	}, nil)

	var capturedTx *txnbuild.Transaction
	signerRouterMock.
		On("SignStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.Transaction"), oldAccount, newAccount).
		Run(func(args mock.Arguments) {
			capturedTx = args.Get(1).(*txnbuild.Transaction)
		}).
		Return(func(ctx context.Context, tx *txnbuild.Transaction, accounts ...schema.TransactionAccount) *txnbuild.Transaction {
			return capturedTx
		}, nil).
		Once()

	signerRouterMock.
		On("SignFeeBumpStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.FeeBumpTransaction"), mock.Anything).
		Return(&txnbuild.FeeBumpTransaction{}, nil).
		Once()

	horizonClientMock.On("SubmitFeeBumpTransactionWithOptions",
		mock.AnythingOfType("*txnbuild.FeeBumpTransaction"),
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	).Return(horizon.Transaction{}, nil)
}

func Test_distributionAccountService_createNewStellarAccountFromAccount(t *testing.T) {
	ctx := context.Background()

	// Create test data (accounts)
	oldAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
	newAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
	hostAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())

	assetUSDC := data.Asset{
		Code:   assets.USDCAssetCode,
		Issuer: assets.USDCAssetIssuerTestnet,
	}

	// Setup common mock functions
	setupSuccessfulBatchInsert := func(signerRouterMock *sigMocks.MockSignerRouter) {
		signerRouterMock.
			On("BatchInsert", mock.Anything, schema.DistributionAccountStellarDBVault, 1).
			Return([]schema.TransactionAccount{newAccount}, nil).
			Once()
	}

	setupBasicBalances := func(distAccServiceMock *mocks.MockDistributionAccountService) {
		balances := map[data.Asset]decimal.Decimal{
			assetUSDC: decimal.NewFromFloat(10.0),
		}
		distAccServiceMock.
			On("GetBalances", mock.Anything, &oldAccount).
			Return(balances, nil).
			Once()
	}

	setupMultipleAssetBalances := func(distAccServiceMock *mocks.MockDistributionAccountService) {
		assetEURO := data.Asset{
			Code:   "EUROC",
			Issuer: "GB3Q6QDZYTHWT7E5PVS3W7FUT5GVAFC5KSZFFLPU25GO7VTC3NM2ZTVO",
		}
		nativeAsset := data.Asset{
			Code:   "XLM",
			Issuer: "",
		}

		balances := map[data.Asset]decimal.Decimal{
			assetUSDC:   decimal.NewFromFloat(10.0),
			assetEURO:   decimal.NewFromFloat(15.0),
			nativeAsset: decimal.NewFromFloat(20.0),
		}
		distAccServiceMock.
			On("GetBalances", mock.Anything, &oldAccount).
			Return(balances, nil).
			Once()
	}

	setupAccountDetail := func(horizonClientMock *horizonclient.MockClient) {
		horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
			AccountID: oldAccount.Address,
		}).Return(horizon.Account{
			AccountID: oldAccount.Address,
			Sequence:  100,
		}, nil)
	}

	setupSuccessfulSigning := func(signerRouterMock *sigMocks.MockSignerRouter, checkOperations func(*txnbuild.Transaction)) {
		var capturedTx *txnbuild.Transaction
		signerRouterMock.
			On("SignStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.Transaction"), oldAccount, newAccount).
			Run(func(args mock.Arguments) {
				capturedTx = args.Get(1).(*txnbuild.Transaction)
				require.NotNil(t, capturedTx)
				if checkOperations != nil {
					checkOperations(capturedTx)
				}
			}).
			Return(func(ctx context.Context, tx *txnbuild.Transaction, accounts ...schema.TransactionAccount) *txnbuild.Transaction {
				return capturedTx
			}, nil).
			Once()
	}

	setupFeeBumpSigning := func(signerRouterMock *sigMocks.MockSignerRouter) {
		signerRouterMock.On("SignFeeBumpStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.FeeBumpTransaction"), hostAccount).
			Return(&txnbuild.FeeBumpTransaction{}, nil).
			Once()
	}

	setupTransactionSubmission := func(horizonClientMock *horizonclient.MockClient, err error) {
		horizonClientMock.On("SubmitFeeBumpTransactionWithOptions",
			mock.AnythingOfType("*txnbuild.FeeBumpTransaction"),
			horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
		).Return(horizon.Transaction{}, err)
	}

	testCases := []struct {
		name       string
		setupMocks func(
			distAccServiceMock *mocks.MockDistributionAccountService,
			horizonClientMock *horizonclient.MockClient,
			signerRouterMock *sigMocks.MockSignerRouter,
		)
		expectedError      string
		expectedNewAccount *schema.TransactionAccount
	}{
		{
			name: "🎉 successfully creates new account and transfers assets",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				setupSuccessfulBatchInsert(signerRouterMock)
				setupBasicBalances(distAccServiceMock)
				setupAccountDetail(horizonClientMock)
				setupSuccessfulSigning(signerRouterMock, nil)
				setupFeeBumpSigning(signerRouterMock)
				setupTransactionSubmission(horizonClientMock, nil)
			},
			expectedError:      "",
			expectedNewAccount: &newAccount,
		},
		{
			name: "🎉 successfully handles multiple assets",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				setupSuccessfulBatchInsert(signerRouterMock)
				setupMultipleAssetBalances(distAccServiceMock)
				setupAccountDetail(horizonClientMock)
				setupSuccessfulSigning(signerRouterMock, func(tx *txnbuild.Transaction) {
					assert.Equal(t, 8, len(tx.Operations()))
				})
				setupFeeBumpSigning(signerRouterMock)
				setupTransactionSubmission(horizonClientMock, nil)
			},
			expectedError:      "",
			expectedNewAccount: &newAccount,
		},
		{
			name: "fails when BatchInsert returns an error",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				signerRouterMock.
					On("BatchInsert", mock.Anything, schema.DistributionAccountStellarDBVault, 1).
					Return(nil, fmt.Errorf("connection error")).
					Once()
			},
			expectedError:      "inserting new account: connection error",
			expectedNewAccount: nil,
		},
		{
			name: "fails when BatchInsert returns no accounts",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				signerRouterMock.
					On("BatchInsert", mock.Anything, schema.DistributionAccountStellarDBVault, 1).
					Return([]schema.TransactionAccount{}, nil).
					Once()
			},
			expectedError:      "expected 1 new account, got 0",
			expectedNewAccount: nil,
		},
		{
			name: "fails when GetBalances returns an error",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				setupSuccessfulBatchInsert(signerRouterMock)
				distAccServiceMock.
					On("GetBalances", mock.Anything, &oldAccount).
					Return(nil, fmt.Errorf("failed to fetch balances")).
					Once()
			},
			expectedError:      "getting old account balances: failed to fetch balances",
			expectedNewAccount: nil,
		},
		{
			name: "fails when AccountDetail returns an error",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				setupSuccessfulBatchInsert(signerRouterMock)
				setupBasicBalances(distAccServiceMock)

				horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
					AccountID: oldAccount.Address,
				}).Return(horizon.Account{}, fmt.Errorf("horizon error")).Once()
			},
			expectedError:      "building and signing transaction: refreshing old account details: horizon error",
			expectedNewAccount: nil,
		},
		{
			name: "fails when SignStellarTransaction returns an error",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				setupSuccessfulBatchInsert(signerRouterMock)
				setupBasicBalances(distAccServiceMock)
				setupAccountDetail(horizonClientMock)

				signerRouterMock.
					On("SignStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.Transaction"), oldAccount, newAccount).
					Return(nil, fmt.Errorf("signing error")).
					Once()
			},
			expectedError:      "building and signing transaction: signing transfer transaction: signing error",
			expectedNewAccount: nil,
		},
		{
			name: "fails when SignFeeBumpStellarTransaction returns an error",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				setupSuccessfulBatchInsert(signerRouterMock)
				setupBasicBalances(distAccServiceMock)
				setupAccountDetail(horizonClientMock)
				setupSuccessfulSigning(signerRouterMock, nil)

				signerRouterMock.On("SignFeeBumpStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.FeeBumpTransaction"), hostAccount).
					Return(nil, fmt.Errorf("fee bump signing error")).
					Once()
			},
			expectedError:      "building and signing transaction: signing fee bump transaction with host account",
			expectedNewAccount: nil,
		},
		{
			name: "fails when SubmitFeeBumpTransactionWithOptions returns an error",
			setupMocks: func(
				distAccServiceMock *mocks.MockDistributionAccountService,
				horizonClientMock *horizonclient.MockClient,
				signerRouterMock *sigMocks.MockSignerRouter,
			) {
				setupSuccessfulBatchInsert(signerRouterMock)
				setupBasicBalances(distAccServiceMock)
				setupAccountDetail(horizonClientMock)
				setupSuccessfulSigning(signerRouterMock, nil)
				setupFeeBumpSigning(signerRouterMock)
				setupTransactionSubmission(horizonClientMock, fmt.Errorf("transaction submission error"))
			},
			expectedError:      "building and signing transaction: submitting account migration transaction: transaction submission error",
			expectedNewAccount: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks
			distAccServiceMock := mocks.NewMockDistributionAccountService(t)
			ledgerTrackerMock := preconditionsMocks.NewMockLedgerNumberTracker(t)
			horizonClientMock := &horizonclient.MockClient{}
			signatureServiceMock, signerRouterMock, distAccResolverMock := signing.NewMockSignatureService(t)

			submitterEngine := engine.SubmitterEngine{
				HorizonClient:       horizonClientMock,
				LedgerNumberTracker: ledgerTrackerMock,
				SignatureService:    signatureServiceMock,
				MaxBaseFee:          150,
			}

			distAccResolverMock.On("HostDistributionAccount").
				Return(hostAccount).
				Maybe()
			ledgerTrackerMock.
				On("GetLedgerBounds").
				Return(&txnbuild.LedgerBounds{MinLedger: 100, MaxLedger: 200}, nil).
				Maybe()

			tc.setupMocks(
				distAccServiceMock,
				horizonClientMock,
				signerRouterMock,
			)

			// Create the service under test
			service := &DistributionAccountService{
				distAccService:             distAccServiceMock,
				submitterEngine:            submitterEngine,
				maxBaseFee:                 100,
				nativeAssetBootstrapAmount: 5,
			}

			// Call the method
			result, err := service.createNewStellarAccountFromAccount(ctx, oldAccount)

			// Assertions
			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedNewAccount, result)
			}

			horizonClientMock.AssertExpectations(t)
		})
	}
}

// failingDeleteKeyService wraps a real DistributionWalletKeyService and fails only DeleteKey,
// standing in for a crash between the on-chain merge and the removal of the superseded key.
type failingDeleteKeyService struct {
	tssSvc.DistributionWalletKeyServiceInterface
	err error
}

func (f failingDeleteKeyService) DeleteKey(_ context.Context, _ string) error {
	return f.err
}

// setupWalletMergeMocks primes the merge/submit path for a rotation whose replacement account is
// minted inside the service (so its address is only known once signing is attempted). The minted
// account is written to mintedAddress.
func setupWalletMergeMocks(
	t *testing.T,
	distAccServiceMock *mocks.MockDistributionAccountService,
	horizonClientMock *horizonclient.MockClient,
	signerRouterMock *sigMocks.MockSignerRouter,
	oldAccount schema.TransactionAccount,
	mintedAddress *string,
) {
	t.Helper()

	distAccServiceMock.
		On("GetBalances", mock.Anything, &oldAccount).
		Return(map[data.Asset]decimal.Decimal{
			{Code: assets.USDCAssetCode, Issuer: assets.USDCAssetIssuerTestnet}: decimal.NewFromFloat(10.0),
		}, nil).
		Once()

	horizonClientMock.On("AccountDetail", horizonclient.AccountRequest{
		AccountID: oldAccount.Address,
	}).Return(horizon.Account{
		AccountID: oldAccount.Address,
		Sequence:  100,
	}, nil).Once()

	var capturedTx *txnbuild.Transaction
	signerRouterMock.
		On("SignStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.Transaction"), oldAccount, mock.AnythingOfType("schema.TransactionAccount")).
		Run(func(args mock.Arguments) {
			capturedTx = args.Get(1).(*txnbuild.Transaction)
			*mintedAddress = args.Get(3).(schema.TransactionAccount).Address
		}).
		Return(func(_ context.Context, _ *txnbuild.Transaction, _ ...schema.TransactionAccount) *txnbuild.Transaction {
			return capturedTx
		}, nil).
		Once()

	signerRouterMock.
		On("SignFeeBumpStellarTransaction", mock.Anything, mock.AnythingOfType("*txnbuild.FeeBumpTransaction"), mock.Anything).
		Return(&txnbuild.FeeBumpTransaction{}, nil).
		Once()

	horizonClientMock.On("SubmitFeeBumpTransactionWithOptions",
		mock.AnythingOfType("*txnbuild.FeeBumpTransaction"),
		horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
	).Return(horizon.Transaction{}, nil).Once()
}

// Test_DistributionAccountService_rotate_multiWallet covers `distribution-account rotate` as a
// multi-wallet operation: the default path must also sync the default wallet row, --wallet-id
// must rotate exactly one non-default wallet through distribution_wallet_keys, and a failure
// after the merge must leave the DB naming an account whose key still exists.
func Test_DistributionAccountService_rotate_multiWallet(t *testing.T) {
	ctx := context.Background()

	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	kek := keypair.MustRandom().Seed()
	walletKeyService, err := tssSvc.NewDistributionWalletKeyService(dbConnectionPool, kek)
	require.NoError(t, err)

	hostAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())

	// The tenant's own distribution account, mirrored by the default wallet row.
	tenantAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
	testTenant := tenant.CreateTenantFixture(t, ctx, dbConnectionPool, "rotate-multiwallet", tenantAccount.Address)
	tenantCtx := sdpcontext.SetTenantInContext(ctx, testTenant)
	tenantManager := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))

	defaultWallet, err := models.DistributionWallets.EnsureDefaultWallet(ctx, dbConnectionPool,
		&tenantAccount.Address, schema.DistributionAccountStellarDBVault, schema.AccountStatusActive)
	require.NoError(t, err)

	// seedWallet creates an ACTIVE non-default wallet backed by a key in distribution_wallet_keys,
	// exactly as DistributionWalletManagementService.CreateWallet leaves it.
	seedWallet := func(t *testing.T, name string) (*data.DistributionWallet, string) {
		t.Helper()

		wallet, wErr := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
			Name:        name,
			AccountType: schema.DistributionAccountStellarDBVault,
		})
		require.NoError(t, wErr)

		kp := keypair.MustRandom()
		require.NoError(t, walletKeyService.StoreKey(ctx, kp.Address(), kp.Seed()))

		wallet, wErr = models.DistributionWallets.UpdateAddress(ctx, dbConnectionPool, wallet.ID, kp.Address())
		require.NoError(t, wErr)
		wallet, wErr = models.DistributionWallets.Activate(ctx, dbConnectionPool, wallet.ID)
		require.NoError(t, wErr)

		return wallet, kp.Address()
	}

	newSubmitterEngine := func(t *testing.T) (engine.SubmitterEngine, *horizonclient.MockClient, *sigMocks.MockSignerRouter, *sigMocks.MockDistributionAccountResolver) {
		t.Helper()

		ledgerTrackerMock := preconditionsMocks.NewMockLedgerNumberTracker(t)
		ledgerTrackerMock.On("GetLedgerBounds").
			Return(&txnbuild.LedgerBounds{MinLedger: 100, MaxLedger: 200}, nil).
			Maybe()

		horizonClientMock := &horizonclient.MockClient{}
		signatureServiceMock, signerRouterMock, distAccResolverMock := signing.NewMockSignatureService(t)
		distAccResolverMock.On("HostDistributionAccount").Return(hostAccount).Maybe()

		return engine.SubmitterEngine{
			HorizonClient:       horizonClientMock,
			LedgerNumberTracker: ledgerTrackerMock,
			SignatureService:    signatureServiceMock,
			MaxBaseFee:          150,
		}, horizonClientMock, signerRouterMock, distAccResolverMock
	}

	t.Run("without --wallet-id the default wallet row is synced before the old key is deleted", func(t *testing.T) {
		submitterEngine, horizonClientMock, signerRouterMock, distAccResolverMock := newSubmitterEngine(t)
		distAccServiceMock := mocks.NewMockDistributionAccountService(t)

		rotatedAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
		distAccResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(tenantAccount, nil).Once()
		signerRouterMock.
			On("BatchInsert", mock.Anything, schema.DistributionAccountStellarDBVault, 1).
			Return([]schema.TransactionAccount{rotatedAccount}, nil).
			Once()
		setupWalletMergeMocks(t, distAccServiceMock, horizonClientMock, signerRouterMock, tenantAccount, new(string))

		// The key delete is the LAST step: asserting the row is already synced when it fails is
		// what pins the ordering requirement.
		signerRouterMock.On("Delete", mock.Anything, tenantAccount).
			Return(fmt.Errorf("vault unavailable")).
			Once()

		service := DistributionAccountService{
			distAccService:             distAccServiceMock,
			submitterEngine:            submitterEngine,
			tenantManager:              tenantManager,
			maxBaseFee:                 100,
			nativeAssetBootstrapAmount: 5,
			models:                     models,
			walletKeyService:           walletKeyService,
		}
		rotateErr := service.rotateDistributionAccount(tenantCtx)
		require.ErrorContains(t, rotateErr, "deleting old account")

		updatedTenant, tErr := tenantManager.GetTenantByID(ctx, testTenant.ID)
		require.NoError(t, tErr)
		assert.Equal(t, rotatedAccount.Address, *updatedTenant.DistributionAccountAddress)

		syncedDefault, dErr := models.DistributionWallets.GetDefault(ctx, dbConnectionPool)
		require.NoError(t, dErr)
		require.NotNil(t, syncedDefault.Address)
		assert.Equal(t, rotatedAccount.Address, *syncedDefault.Address, "default wallet row must be synced before the key delete")
		assert.Equal(t, defaultWallet.ID, syncedDefault.ID)

		horizonClientMock.AssertExpectations(t)
	})

	t.Run("--wallet-id rotates that wallet only, through distribution_wallet_keys", func(t *testing.T) {
		wallet, oldAddress := seedWallet(t, "wallet-rotate-ok")
		oldAccount := schema.TransactionAccount{
			Address: oldAddress,
			Type:    wallet.AccountType,
			Status:  wallet.AccountStatus,
		}

		submitterEngine, horizonClientMock, signerRouterMock, distAccResolverMock := newSubmitterEngine(t)
		distAccServiceMock := mocks.NewMockDistributionAccountService(t)

		distAccResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(tenantAccount, nil).Once()
		var mintedAddress string
		setupWalletMergeMocks(t, distAccServiceMock, horizonClientMock, signerRouterMock, oldAccount, &mintedAddress)

		service := DistributionAccountService{
			distAccService:             distAccServiceMock,
			submitterEngine:            submitterEngine,
			tenantManager:              tenantManager,
			maxBaseFee:                 100,
			nativeAssetBootstrapAmount: 5,
			models:                     models,
			walletKeyService:           walletKeyService,
			walletID:                   wallet.ID,
		}
		require.NoError(t, service.rotateDistributionAccount(tenantCtx))
		require.NotEmpty(t, mintedAddress)
		require.NotEqual(t, oldAddress, mintedAddress)

		// The wallet row points at the freshly minted account.
		rotated, gErr := models.DistributionWallets.Get(ctx, dbConnectionPool, wallet.ID)
		require.NoError(t, gErr)
		require.NotNil(t, rotated.Address)
		assert.Equal(t, mintedAddress, *rotated.Address)
		assert.Equal(t, data.ActiveDistributionWalletStatus, rotated.Status)

		// The replacement key is stored per-wallet under its own DEK, and decrypts to the minted
		// account's seed.
		seed, kErr := walletKeyService.GetPrivateKey(ctx, mintedAddress)
		require.NoError(t, kErr)
		kp, pErr := keypair.ParseFull(seed)
		require.NoError(t, pErr)
		assert.Equal(t, mintedAddress, kp.Address())

		// The superseded key is gone.
		_, kErr = walletKeyService.GetPrivateKey(ctx, oldAddress)
		require.ErrorIs(t, kErr, tssStore.ErrRecordNotFound)

		// Neither the tenant nor the default wallet moved.
		unchangedTenant, tErr := tenantManager.GetTenantByID(ctx, testTenant.ID)
		require.NoError(t, tErr)
		assert.NotEqual(t, mintedAddress, *unchangedTenant.DistributionAccountAddress)
		untouchedDefault, dErr := models.DistributionWallets.GetDefault(ctx, dbConnectionPool)
		require.NoError(t, dErr)
		require.NotNil(t, untouchedDefault.Address)
		assert.NotEqual(t, mintedAddress, *untouchedDefault.Address)

		horizonClientMock.AssertExpectations(t)
	})

	t.Run("a failure between the merge and the key delete leaves the wallet row consistent", func(t *testing.T) {
		wallet, oldAddress := seedWallet(t, "wallet-rotate-crash")
		oldAccount := schema.TransactionAccount{
			Address: oldAddress,
			Type:    wallet.AccountType,
			Status:  wallet.AccountStatus,
		}

		submitterEngine, horizonClientMock, signerRouterMock, distAccResolverMock := newSubmitterEngine(t)
		distAccServiceMock := mocks.NewMockDistributionAccountService(t)

		distAccResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(tenantAccount, nil).Once()
		var mintedAddress string
		setupWalletMergeMocks(t, distAccServiceMock, horizonClientMock, signerRouterMock, oldAccount, &mintedAddress)

		service := DistributionAccountService{
			distAccService:             distAccServiceMock,
			submitterEngine:            submitterEngine,
			tenantManager:              tenantManager,
			maxBaseFee:                 100,
			nativeAssetBootstrapAmount: 5,
			models:                     models,
			walletKeyService: failingDeleteKeyService{
				DistributionWalletKeyServiceInterface: walletKeyService,
				err:                                   fmt.Errorf("key store unavailable"),
			},
			walletID: wallet.ID,
		}
		rotateErr := service.rotateDistributionAccount(tenantCtx)
		require.ErrorContains(t, rotateErr, "deleting old account")
		require.NotEmpty(t, mintedAddress)

		// The row already names the merged-into account, and that account's key is present, so
		// the wallet is still usable and re-running the command is safe.
		rotated, gErr := models.DistributionWallets.Get(ctx, dbConnectionPool, wallet.ID)
		require.NoError(t, gErr)
		require.NotNil(t, rotated.Address)
		assert.Equal(t, mintedAddress, *rotated.Address)

		_, kErr := walletKeyService.GetPrivateKey(ctx, mintedAddress)
		require.NoError(t, kErr)

		// The old key survives the failure — nothing is lost, it is simply not cleaned up yet.
		_, kErr = walletKeyService.GetPrivateKey(ctx, oldAddress)
		require.NoError(t, kErr)

		horizonClientMock.AssertExpectations(t)
	})

	t.Run("rejects wallets that cannot be rotated", func(t *testing.T) {
		pendingWallet, pErr := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
			Name:        "wallet-rotate-pending",
			AccountType: schema.DistributionAccountStellarDBVault,
		})
		require.NoError(t, pErr)

		archivedWallet, aAddress := seedWallet(t, "wallet-rotate-archived")
		_, aErr := models.DistributionWallets.Archive(ctx, dbConnectionPool, archivedWallet.ID)
		require.NoError(t, aErr)

		circleWallet, cErr := models.DistributionWallets.Insert(ctx, dbConnectionPool, data.DistributionWalletInsert{
			Name:        "wallet-rotate-circle",
			AccountType: schema.DistributionAccountCircleDBVault,
		})
		require.NoError(t, cErr)
		circleWallet, cErr = models.DistributionWallets.UpdateAddress(ctx, dbConnectionPool, circleWallet.ID, keypair.MustRandom().Address())
		require.NoError(t, cErr)
		circleWallet, cErr = models.DistributionWallets.Activate(ctx, dbConnectionPool, circleWallet.ID)
		require.NoError(t, cErr)

		testCases := []struct {
			name          string
			walletID      string
			expectedError string
		}{
			{
				name:          "unknown wallet",
				walletID:      "11111111-1111-1111-1111-111111111111",
				expectedError: "getting distribution wallet",
			},
			{
				name:          "pending wallet",
				walletID:      pendingWallet.ID,
				expectedError: "only ACTIVE distribution wallets can be rotated",
			},
			{
				name:          "archived wallet",
				walletID:      archivedWallet.ID,
				expectedError: "only ACTIVE distribution wallets can be rotated",
			},
			{
				name:          "non-Stellar wallet",
				walletID:      circleWallet.ID,
				expectedError: "rotation is only supported for DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT wallets",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				submitterEngine, horizonClientMock, _, distAccResolverMock := newSubmitterEngine(t)
				distAccResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(tenantAccount, nil).Once()

				service := DistributionAccountService{
					distAccService:             mocks.NewMockDistributionAccountService(t),
					submitterEngine:            submitterEngine,
					tenantManager:              tenantManager,
					maxBaseFee:                 100,
					nativeAssetBootstrapAmount: 5,
					models:                     models,
					walletKeyService:           walletKeyService,
					walletID:                   tc.walletID,
				}
				require.ErrorContains(t, service.rotateDistributionAccount(tenantCtx), tc.expectedError)
				horizonClientMock.AssertExpectations(t)
			})
		}

		// The archived wallet's key is untouched by the rejection.
		_, kErr := walletKeyService.GetPrivateKey(ctx, aAddress)
		require.NoError(t, kErr)
	})

	t.Run("--wallet-id naming the default wallet takes the tenant-account path", func(t *testing.T) {
		currentDefault, dErr := models.DistributionWallets.GetDefault(ctx, dbConnectionPool)
		require.NoError(t, dErr)
		require.NotNil(t, currentDefault.Address)
		currentAccount := schema.NewDefaultStellarTransactionAccount(*currentDefault.Address)

		submitterEngine, horizonClientMock, signerRouterMock, distAccResolverMock := newSubmitterEngine(t)
		distAccServiceMock := mocks.NewMockDistributionAccountService(t)

		rotatedAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
		distAccResolverMock.On("DistributionAccountFromContext", mock.Anything).Return(currentAccount, nil).Once()
		signerRouterMock.
			On("BatchInsert", mock.Anything, schema.DistributionAccountStellarDBVault, 1).
			Return([]schema.TransactionAccount{rotatedAccount}, nil).
			Once()
		setupWalletMergeMocks(t, distAccServiceMock, horizonClientMock, signerRouterMock, currentAccount, new(string))
		signerRouterMock.On("Delete", mock.Anything, currentAccount).Return(nil).Once()

		service := DistributionAccountService{
			distAccService:             distAccServiceMock,
			submitterEngine:            submitterEngine,
			tenantManager:              tenantManager,
			maxBaseFee:                 100,
			nativeAssetBootstrapAmount: 5,
			models:                     models,
			walletKeyService:           walletKeyService,
			walletID:                   currentDefault.ID,
		}
		require.NoError(t, service.rotateDistributionAccount(tenantCtx))

		updatedTenant, tErr := tenantManager.GetTenantByID(ctx, testTenant.ID)
		require.NoError(t, tErr)
		assert.Equal(t, rotatedAccount.Address, *updatedTenant.DistributionAccountAddress)
		syncedDefault, sErr := models.DistributionWallets.GetDefault(ctx, dbConnectionPool)
		require.NoError(t, sErr)
		require.NotNil(t, syncedDefault.Address)
		assert.Equal(t, rotatedAccount.Address, *syncedDefault.Address)

		horizonClientMock.AssertExpectations(t)
	})
}
