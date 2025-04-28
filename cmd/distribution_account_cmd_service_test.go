package cmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_DistAccCmdService_RotateDistributionAccount(t *testing.T) {
	ctx := context.Background()

	dbConnectionPool := testutils.OpenTestDBConnectionPool(t)

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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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
			ctx = tenant.SaveTenantInContext(ctx, testTenant)

			service := DistributionAccountService{
				distAccService:             distAccServiceMock,
				submitterEngine:            submitterEngine,
				tenantManager:              tenantManager,
				maxBaseFee:                 100,
				nativeAssetBootstrapAmount: 5,
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

	balances := map[data.Asset]float64{
		assetUSDC: 10.0,
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
		balances := map[data.Asset]float64{
			assetUSDC: 10.0,
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

		balances := map[data.Asset]float64{
			assetUSDC:   10.0,
			assetEURO:   15.0,
			nativeAsset: 20.0,
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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
