package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/protocols/horizon/base"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_CreateChannelAccountsOnChain(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)

	currLedgerNumber := 100
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNumber + preconditions.IncrementForMaxLedgerBounds),
	}

	hostAccountKP := keypair.MustRandom()
	hostAccount := schema.NewDefaultHostAccount(hostAccountKP.Address())
	chAccEncrypterPass := keypair.MustRandom().Seed()
	dbVaultEncrypterPass := keypair.MustRandom().Seed()

	testCases := []struct {
		name                 string
		numOfChanAccToCreate int
		prepareMocksFn       func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, privateKeyEncrypterMock *sdpUtils.PrivateKeyEncrypterMock)
		wantErrContains      string
	}{
		{
			name:                 "returns error when 'numOfChanAccToCreate > MaximumCreateAccountOperationsPerStellarTx'",
			numOfChanAccToCreate: MaximumCreateAccountOperationsPerStellarTx + 1,
			wantErrContains:      "cannot create more than 19 channel accounts",
		},
		{
			name:                 "returns error when numOfChanAccToCreate=0",
			numOfChanAccToCreate: 0,
			wantErrContains:      ErrInvalidNumOfChannelAccountsToCreate.Error(),
		},
		{
			name:                 "returns error when numOfChanAccToCreate=-2",
			numOfChanAccToCreate: -2,
			wantErrContains:      ErrInvalidNumOfChannelAccountsToCreate.Error(),
		},
		{
			name:                 "returns error when HorizonClient fails getting AccountDetails",
			numOfChanAccToCreate: 2,
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, _ *preconditionsMocks.MockLedgerNumberTracker, _ *sdpUtils.PrivateKeyEncrypterMock) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{}, horizonclient.Error{
						Problem: problem.NotFound,
					}).
					Once()
			},
			wantErrContains: "failed to retrieve host account: horizon response error: StatusCode=404, Type=not_found, Title=Resource Missing, Detail=The resource at the url requested was not found.  This usually occurs for one of two reasons:  The url requested is not valid, or no data in our database could be found with the parameters provided.",
		},
		{
			name:                 "returns error when fails to retrieve ledger bounds",
			numOfChanAccToCreate: 2,
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, _ *sdpUtils.PrivateKeyEncrypterMock) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{
						AccountID: hostAccount.Address,
						Sequence:  1,
					}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").
					Return(nil, fmt.Errorf("unexpected error")).
					Once()
			},
			wantErrContains: "failed to get ledger bounds: unexpected error",
		},
		{
			name:                 "returns error when fails encrypting private key",
			numOfChanAccToCreate: 2,
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, privateKeyEncrypterMock *sdpUtils.PrivateKeyEncrypterMock) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{
						AccountID: hostAccount.Address,
						Sequence:  1,
					}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").Return(ledgerBounds, nil).Once().
					On("GetLedgerNumber").Return(currLedgerNumber, nil).Once()
				privateKeyEncrypterMock.
					On("Encrypt", mock.AnythingOfType("string"), chAccEncrypterPass).
					Return("", errors.New("unexpected error")).
					Once()
			},
			wantErrContains: "encrypting channel account private key: unexpected error",
		},
		{
			name:                 "returns error when fails submitting transaction to horizon",
			numOfChanAccToCreate: 2,
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, privateKeyEncrypterMock *sdpUtils.PrivateKeyEncrypterMock) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{
						AccountID: hostAccount.Address,
						Sequence:  1,
					}, nil).
					Once().
					On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, horizonclient.Error{
						Problem: problem.P{
							Type:   "https://stellar.org/horizon-errors/timeout",
							Title:  "Timeout",
							Detail: "Foo bar detail",
							Status: http.StatusRequestTimeout,
							Extras: map[string]interface{}{"foo": "bar"},
						},
					}).
					Once()

				mLedgerNumberTracker.
					On("GetLedgerBounds").Return(ledgerBounds, nil).Once().
					On("GetLedgerNumber").Return(currLedgerNumber, nil).Times(3)

				privateKeyEncrypterMock.
					On("Encrypt", mock.AnythingOfType("string"), chAccEncrypterPass).Return("encryptedkey", nil).Twice().
					On("Decrypt", mock.AnythingOfType("string"), chAccEncrypterPass).Return(keypair.MustRandom().Seed(), nil).Twice()
			},
			wantErrContains: "creating sponsored channel accounts: horizon response error: StatusCode=408, Type=https://stellar.org/horizon-errors/timeout, Title=Timeout, Detail=Foo bar detail",
		},
		{
			name:                 "🎉 successfully creates channel accounts on-chain (ENCRYPTED)",
			numOfChanAccToCreate: 3,
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, privateKeyEncrypterMock *sdpUtils.PrivateKeyEncrypterMock) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{
						AccountID: hostAccount.Address,
						Sequence:  1,
					}, nil).
					Once().
					On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, nil).
					Once()

				mLedgerNumberTracker.
					On("GetLedgerBounds").Return(ledgerBounds, nil).Once().
					On("GetLedgerNumber").Return(currLedgerNumber, nil).Once()

				privateKeyEncrypterMock.
					On("Encrypt", mock.AnythingOfType("string"), chAccEncrypterPass).Return("encryptedkey", nil).Times(3).
					On("Decrypt", mock.AnythingOfType("string"), chAccEncrypterPass).Return(keypair.MustRandom().Seed(), nil).Times(3)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)

			count, err := chAccModel.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, 0, count)

			// Prepare mocks
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			horizonClientMock := &horizonclient.MockClient{}
			privateKeyEncrypterMock := &sdpUtils.PrivateKeyEncrypterMock{}
			mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
			mDistAccResolver.
				On("HostDistributionAccount").
				Return(hostAccount).
				Maybe()
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(horizonClientMock, mLedgerNumberTracker, privateKeyEncrypterMock)
			}

			sigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
				NetworkPassphrase:           network.TestNetworkPassphrase,
				DBConnectionPool:            dbConnectionPool,
				DistributionPrivateKey:      hostAccountKP.Seed(),
				ChAccEncryptionPassphrase:   chAccEncrypterPass,
				Encrypter:                   privateKeyEncrypterMock,
				LedgerNumberTracker:         mLedgerNumberTracker,
				DistAccEncryptionPassphrase: dbVaultEncrypterPass,
				DistributionAccountResolver: mDistAccResolver,
			})
			require.NoError(t, err)

			submitterEngine := engine.SubmitterEngine{
				HorizonClient:       horizonClientMock,
				SignatureService:    sigService,
				MaxBaseFee:          txnbuild.MinBaseFee,
				LedgerNumberTracker: mLedgerNumberTracker,
			}

			channelAccountAddresses, err := CreateChannelAccountsOnChain(ctx, submitterEngine, tc.numOfChanAccToCreate)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Empty(t, channelAccountAddresses)
				assert.ErrorContains(t, err, tc.wantErrContains)

				count, err = chAccModel.Count(ctx)
				require.NoError(t, err)
				assert.Equal(t, 0, count)
			} else {
				require.NoError(t, err)
				assert.Len(t, channelAccountAddresses, tc.numOfChanAccToCreate)

				count, err = chAccModel.Count(ctx)
				require.NoError(t, err)
				assert.Equal(t, tc.numOfChanAccToCreate, count)

				allChAcc, err := chAccModel.GetAll(ctx, dbConnectionPool, math.MaxInt32, 100)
				require.NoError(t, err)
				assert.Len(t, allChAcc, tc.numOfChanAccToCreate)

				for _, chAcc := range allChAcc {
					assert.False(t, strkey.IsValidEd25519SecretSeed(chAcc.PrivateKey))
				}
			}

			horizonClientMock.AssertExpectations(t)
			privateKeyEncrypterMock.AssertExpectations(t)
		})
	}
}

func Test_DeleteChannelAccountOnChain(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	hostAccountKP := keypair.MustRandom()
	hostAccount := schema.NewDefaultHostAccount(hostAccountKP.Address())

	currLedger := 100
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedger + preconditions.IncrementForMaxLedgerBounds),
	}

	chAccAddress := keypair.MustRandom().Address()
	chTxAcc := schema.NewDefaultChannelAccount(chAccAddress)

	testCases := []struct {
		name                 string
		prepareMocksFn       func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, sigRouter *sigMocks.MockSignerRouter)
		chAccAddressToDelete string
		wantErrContains      string
	}{
		{
			name: "returns error when HorizonClient fails getting AccountDetails",
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, _ *preconditionsMocks.MockLedgerNumberTracker, _ *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{}, horizonclient.Error{Problem: problem.NotFound}).
					Once()
			},
			wantErrContains: `retrieving host account from distribution seed: horizon error: "Resource Missing" - check horizon.Error.Problem for more information`,
		},
		{
			name: "returns error when GetLedgerBounds fails",
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, _ *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{AccountID: hostAccount.Address, Sequence: 1}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").
					Return(nil, fmt.Errorf("unexpected error")).
					Once()
			},
			wantErrContains: "failed to get ledger bounds: unexpected error",
		},
		{
			name:                 "returns error when channel account doesnt exist",
			chAccAddressToDelete: chAccAddress,
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, sigRouter *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{
						AccountID: hostAccount.Address,
						Sequence:  1,
					}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").
					Return(ledgerBounds, nil).
					Once()
				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), chTxAcc, hostAccount).
					Return(nil, fmt.Errorf("signing remove account transaction for account")).
					Once()
			},
			wantErrContains: "signing remove account transaction for account",
		},
		{
			name:                 "returns error when fails submitting transaction to horizon",
			chAccAddressToDelete: chAccAddress,
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, sigRouter *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{
						AccountID: hostAccount.Address,
						Sequence:  1,
					}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").
					Return(ledgerBounds, nil).
					Once()
				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), chTxAcc, hostAccount).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				horizonClientMock.On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, horizonclient.Error{
						Problem: problem.P{
							Type:   "https://stellar.org/horizon-errors/timeout",
							Title:  "Timeout",
							Status: http.StatusRequestTimeout,
						},
					}).
					Once()
			},
			wantErrContains: fmt.Sprintf(
				`submitting remove account transaction to the network for account %s: horizon response error: StatusCode=408, Type=https://stellar.org/horizon-errors/timeout, Title=Timeout`,
				chAccAddress,
			),
		},
		{
			name:                 "🎉 Successfully deletes channel account on chain and database",
			chAccAddressToDelete: chAccAddress,
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, mLedgerNumberTracker *preconditionsMocks.MockLedgerNumberTracker, sigRouter *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{
						AccountID: hostAccount.Address,
						Sequence:  1,
					}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").
					Return(ledgerBounds, nil).
					Once()
				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), chTxAcc, hostAccount).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				horizonClientMock.On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, nil).
					Once()
				sigRouter.On("Delete", ctx, chTxAcc).Return(nil).Once()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)

			// prepare mocks
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			horizonClientMock := &horizonclient.MockClient{}
			privateKeyEncrypterMock := &sdpUtils.PrivateKeyEncrypterMock{}
			sigService, sigRouter, mDistAccResolver := signing.NewMockSignatureService(t)
			mDistAccResolver.
				On("HostDistributionAccount").
				Return(hostAccount)
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(horizonClientMock, mLedgerNumberTracker, sigRouter)
			}

			submitterEngine := engine.SubmitterEngine{
				HorizonClient:       horizonClientMock,
				SignatureService:    sigService,
				MaxBaseFee:          txnbuild.MinBaseFee,
				LedgerNumberTracker: mLedgerNumberTracker,
			}

			err = DeleteChannelAccountOnChain(ctx, submitterEngine, tc.chAccAddressToDelete)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}

			horizonClientMock.AssertExpectations(t)
			privateKeyEncrypterMock.AssertExpectations(t)
		})
	}
}

func Test_FundDistributionAccount(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	srcAccAddress := keypair.MustRandom().Address()
	hostAccount := schema.NewDefaultHostAccount(srcAccAddress)
	dstAccAddress := keypair.MustRandom().Address()
	tenantDistributionFundingAmount := tenant.MinTenantDistributionAccountAmount
	require.NoError(t, err)

	testCases := []struct {
		name            string
		prepareMocksFn  func(horizonClientMock *horizonclient.MockClient, sigRouter *sigMocks.MockSignerRouter)
		amountToFund    int
		srcAccAddress   string
		dstAccAddress   string
		wantErrContains string
	}{
		{
			name:            "source account is the same as destination account",
			amountToFund:    tenantDistributionFundingAmount,
			srcAccAddress:   srcAccAddress,
			dstAccAddress:   srcAccAddress,
			wantErrContains: "source account and destination account cannot be the same",
		},
		{
			name: "returns error when HorizonClient fails getting host distribution AccountDetails",
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, _ *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{AccountID: hostAccount.Address}, horizonclient.Error{Problem: problem.NotFound}).
					Times(CreateAndFundAccountRetryAttempts)
			},
			amountToFund:  tenantDistributionFundingAmount,
			srcAccAddress: srcAccAddress,
			dstAccAddress: dstAccAddress,
			wantErrContains: fmt.Sprintf(
				`getting details for source account: cannot find account on the network %s: horizon error: "Resource Missing" - check horizon.Error.Problem for more information`,
				srcAccAddress,
			),
		},
		{
			name: "returns error when failing to sign raw transaction",
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, sigRouter *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{AccountID: hostAccount.Address}, nil).
					Times(CreateAndFundAccountRetryAttempts)
				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostAccount).
					Return(&txnbuild.Transaction{}, errors.New("failed to sign raw tx")).
					Times(CreateAndFundAccountRetryAttempts)
			},
			amountToFund:  tenantDistributionFundingAmount,
			srcAccAddress: srcAccAddress,
			dstAccAddress: dstAccAddress,
			wantErrContains: fmt.Sprintf(
				`signing create account tx for account %s:`,
				dstAccAddress,
			),
		},
		{
			name: "returns error when failing to submit tx over Horizon - timeout",
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, sigRouter *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{AccountID: hostAccount.Address}, nil).
					Times(CreateAndFundAccountRetryAttempts)
				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostAccount).
					Return(&txnbuild.Transaction{}, nil).
					Times(CreateAndFundAccountRetryAttempts)
				horizonClientMock.On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, horizonclient.Error{
						Problem: problem.P{
							Type:   "https://stellar.org/horizon-errors/timeout",
							Title:  "Timeout",
							Status: http.StatusRequestTimeout,
							Extras: map[string]interface{}{
								"result_codes": map[string]interface{}{},
							},
						},
					}).
					Times(CreateAndFundAccountRetryAttempts)
			},
			amountToFund:    tenantDistributionFundingAmount,
			srcAccAddress:   srcAccAddress,
			dstAccAddress:   dstAccAddress,
			wantErrContains: "maximum number of retries reached or terminal error encountered",
		},
		{
			name: "returns error when failing to submit tx over Horizon - insufficient balance",
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, sigRouter *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{AccountID: hostAccount.Address}, nil).
					Once()
				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostAccount).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				horizonClientMock.On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, horizonclient.Error{
						Problem: problem.P{
							Status: http.StatusBadRequest,
							Extras: map[string]interface{}{
								"result_codes": map[string]interface{}{
									"transaction": "tx_insufficient_balance",
								},
							},
						},
					}).
					Once()
			},
			amountToFund:    tenantDistributionFundingAmount,
			srcAccAddress:   srcAccAddress,
			dstAccAddress:   dstAccAddress,
			wantErrContains: "maximum number of retries reached or terminal error encountered",
		},
		{
			name: "successfully creates and funds tenant distribution account",
			prepareMocksFn: func(horizonClientMock *horizonclient.MockClient, sigRouter *sigMocks.MockSignerRouter) {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostAccount.Address}).
					Return(horizon.Account{AccountID: hostAccount.Address}, nil).
					Once()
				sigRouter.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostAccount).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				horizonClientMock.On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, nil).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: dstAccAddress}).
					Return(horizon.Account{AccountID: dstAccAddress}, nil).
					Once()
			},
			amountToFund:  tenantDistributionFundingAmount,
			srcAccAddress: srcAccAddress,
			dstAccAddress: dstAccAddress,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
			sigService, sigRouter, mDistAccResolver := signing.NewMockSignatureService(t)
			mDistAccResolver.
				On("HostDistributionAccount").
				Return(hostAccount)
			horizonClientMock := &horizonclient.MockClient{}

			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn(horizonClientMock, sigRouter)
			}

			submitterEngine := engine.SubmitterEngine{
				HorizonClient:       horizonClientMock,
				SignatureService:    sigService,
				MaxBaseFee:          txnbuild.MinBaseFee,
				LedgerNumberTracker: mLedgerNumberTracker,
			}

			err = CreateAndFundAccount(ctx, submitterEngine, tc.amountToFund, tc.srcAccAddress, tc.dstAccAddress)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
			} else {
				require.NoError(t, err)
			}

			horizonClientMock.AssertExpectations(t)
		})
	}
}

func newSubmitterEngineForTrust(t *testing.T) (engine.SubmitterEngine, *horizonclient.MockClient, *sigMocks.MockSignerRouter) {
	t.Helper()

	ledgerTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	hClient := &horizonclient.MockClient{}
	sigService, sigRouter, _ := signing.NewMockSignatureService(t)

	return engine.SubmitterEngine{
		HorizonClient:       hClient,
		SignatureService:    sigService,
		LedgerNumberTracker: ledgerTracker,
		MaxBaseFee:          2 * txnbuild.MinBaseFee,
	}, hClient, sigRouter
}

func TestAddTrustlines_AddsTrustlines(t *testing.T) {
	ctx := context.Background()
	issuerKP := keypair.MustRandom()
	accountAddress := keypair.MustRandom().Address()
	account := schema.TransactionAccount{
		Address: accountAddress,
		Type:    schema.DistributionAccountStellarDBVault,
		Status:  schema.AccountStatusActive,
	}
	assets := []data.Asset{
		{Code: "USDC", Issuer: issuerKP.Address()},
		{Code: "NGNT", Issuer: keypair.MustRandom().Address()},
	}

	submitterEngine, hClient, sigRouter := newSubmitterEngineForTrust(t)

	hClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: accountAddress}).
		Return(horizon.Account{AccountID: accountAddress, Sequence: 123}, nil).
		Once()

	sigRouter.
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), account).
		Return(&txnbuild.Transaction{}, nil).
		Once()

	hClient.
		On(
			"SubmitTransactionWithOptions",
			mock.AnythingOfType("*txnbuild.Transaction"),
			horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
		).
		Return(horizon.Transaction{}, nil).
		Once()

	count, err := AddTrustlines(ctx, submitterEngine, account, assets)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	hClient.AssertExpectations(t)
	sigRouter.AssertExpectations(t)
}

func TestAddTrustlines_SkipsExistingTrustlines(t *testing.T) {
	ctx := context.Background()
	issuer := keypair.MustRandom().Address()
	accountAddress := keypair.MustRandom().Address()
	account := schema.TransactionAccount{Address: accountAddress, Type: schema.DistributionAccountStellarDBVault, Status: schema.AccountStatusActive}
	assets := []data.Asset{{Code: "USDC", Issuer: issuer}}

	submitterEngine, hClient, sigRouter := newSubmitterEngineForTrust(t)

	hClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: accountAddress}).
		Return(horizon.Account{
			AccountID: accountAddress,
			Sequence:  123,
			Balances: []horizon.Balance{
				{Asset: base.Asset{Type: "credit_alphanum4", Code: "USDC", Issuer: issuer}},
			},
		}, nil).
		Once()

	count, err := AddTrustlines(ctx, submitterEngine, account, assets)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	sigRouter.AssertNumberOfCalls(t, "SignStellarTransaction", 0)
	hClient.AssertNotCalled(t, "SubmitTransactionWithOptions", mock.Anything, mock.Anything)

	hClient.AssertExpectations(t)
	sigRouter.AssertExpectations(t)
}

func TestAddTrustlines_SubmitFailure(t *testing.T) {
	ctx := context.Background()
	issuer := keypair.MustRandom().Address()
	accountAddress := keypair.MustRandom().Address()
	account := schema.TransactionAccount{Address: accountAddress, Type: schema.DistributionAccountStellarDBVault, Status: schema.AccountStatusActive}
	assets := []data.Asset{{Code: "USDC", Issuer: issuer}}

	submitterEngine, hClient, sigRouter := newSubmitterEngineForTrust(t)

	hClient.
		On("AccountDetail", horizonclient.AccountRequest{AccountID: accountAddress}).
		Return(horizon.Account{AccountID: accountAddress, Sequence: 42}, nil).
		Once()

	sigRouter.
		On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), account).
		Return(&txnbuild.Transaction{}, nil).
		Once()

	submissionErr := errors.New("boom")
	hClient.
		On(
			"SubmitTransactionWithOptions",
			mock.AnythingOfType("*txnbuild.Transaction"),
			horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true},
		).
		Return(horizon.Transaction{}, submissionErr).
		Once()

	count, err := AddTrustlines(ctx, submitterEngine, account, assets)
	assert.ErrorContains(t, err, "submitting change trust transaction to network")
	assert.Equal(t, 0, count)

	hClient.AssertExpectations(t)
	sigRouter.AssertExpectations(t)
}
