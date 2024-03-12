package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	preconditionsMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

func Test_CreateChannelAccountsOnChain(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	horizonClientMock := &horizonclient.MockClient{}
	privateKeyEncrypterMock := &utils.PrivateKeyEncrypterMock{}
	ctx := context.Background()
	chAccModel := store.NewChannelAccountModel(dbConnectionPool)

	currLedgerNumber := 100
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedgerNumber + preconditions.IncrementForMaxLedgerBounds),
	}

	distributionKP := keypair.MustRandom()
	encrypterPass := distributionKP.Seed()

	mDistAccResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistAccResolver.
		On("HostDistributionAccount").
		Return(distributionKP.Address())

	sigService, err := signing.NewSignatureService(signing.SignatureServiceOptions{
		DistributionSignerType:      signing.DistributionAccountEnvSignatureClientType,
		NetworkPassphrase:           network.TestNetworkPassphrase,
		DBConnectionPool:            dbConnectionPool,
		DistributionPrivateKey:      distributionKP.Seed(),
		ChAccEncryptionPassphrase:   encrypterPass,
		Encrypter:                   privateKeyEncrypterMock,
		LedgerNumberTracker:         mLedgerNumberTracker,
		DistributionAccountResolver: mDistAccResolver,
	})
	require.NoError(t, err)

	testCases := []struct {
		name                 string
		numOfChanAccToCreate int
		prepareMocksFn       func()
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
			prepareMocksFn: func() {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: sigService.HostDistributionAccount()}).
					Return(horizon.Account{}, horizonclient.Error{
						Problem: problem.NotFound,
					}).
					Once()
			},
			wantErrContains: "failed to retrieve root account: horizon response error: StatusCode=404, Type=not_found, Title=Resource Missing, Detail=The resource at the url requested was not found.  This usually occurs for one of two reasons:  The url requested is not valid, or no data in our database could be found with the parameters provided.",
		},
		{
			name:                 "returns error when fails to retrieve ledger bounds",
			numOfChanAccToCreate: 2,
			prepareMocksFn: func() {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: sigService.HostDistributionAccount()}).
					Return(horizon.Account{
						AccountID: sigService.HostDistributionAccount(),
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
			prepareMocksFn: func() {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: sigService.HostDistributionAccount()}).
					Return(horizon.Account{
						AccountID: sigService.HostDistributionAccount(),
						Sequence:  1,
					}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").Return(ledgerBounds, nil).Once().
					On("GetLedgerNumber").Return(currLedgerNumber, nil).Once()
				privateKeyEncrypterMock.
					On("Encrypt", mock.AnythingOfType("string"), encrypterPass).
					Return("", errors.New("unexpected error")).
					Once()
			},
			wantErrContains: "encrypting channel account private key: unexpected error",
		},
		{
			name:                 "returns error when fails submitting transaction to horizon",
			numOfChanAccToCreate: 2,
			prepareMocksFn: func() {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: sigService.HostDistributionAccount()}).
					Return(horizon.Account{
						AccountID: sigService.HostDistributionAccount(),
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
					On("Encrypt", mock.AnythingOfType("string"), encrypterPass).Return("encryptedkey", nil).Twice().
					On("Decrypt", mock.AnythingOfType("string"), encrypterPass).Return(keypair.MustRandom().Seed(), nil).Twice()
			},
			wantErrContains: "creating sponsored channel accounts: horizon response error: StatusCode=408, Type=https://stellar.org/horizon-errors/timeout, Title=Timeout, Detail=Foo bar detail",
		},
		{
			name:                 "🎉 successfully creates channel accounts on-chain (ENCRYPTED)",
			numOfChanAccToCreate: 3,
			prepareMocksFn: func() {
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: distributionKP.Address()}).
					Return(horizon.Account{
						AccountID: distributionKP.Address(),
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
					On("Encrypt", mock.AnythingOfType("string"), encrypterPass).Return("encryptedkey", nil).Times(3).
					On("Decrypt", mock.AnythingOfType("string"), encrypterPass).Return(keypair.MustRandom().Seed(), nil).Times(3)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			count, err := chAccModel.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, 0, count)

			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn()
			}

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

			store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}

	horizonClientMock.AssertExpectations(t)
	privateKeyEncrypterMock.AssertExpectations(t)
}

func Test_DeleteChannelAccountOnChain(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	horizonClientMock := &horizonclient.MockClient{}
	privateKeyEncrypterMock := &utils.PrivateKeyEncrypterMock{}
	ctx := context.Background()

	distributionKP := keypair.MustRandom()
	distributionAddress := distributionKP.Address()
	sigService, mChAccSigClient, _, mHostAccSigClient, mDistAccResolver := signing.NewMockSignatureService(t)

	currLedger := 100
	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)
	ledgerBounds := &txnbuild.LedgerBounds{
		MaxLedger: uint32(currLedger + preconditions.IncrementForMaxLedgerBounds),
	}

	chAccAddress := keypair.MustRandom().Address()

	testCases := []struct {
		name                 string
		prepareMocksFn       func()
		chAccAddressToDelete string
		wantErrContains      string
	}{
		{
			name: "returns error when HorizonClient fails getting AccountDetails",
			prepareMocksFn: func() {
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(distributionAddress).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: distributionAddress}).
					Return(horizon.Account{}, horizonclient.Error{Problem: problem.NotFound}).
					Once()
			},
			wantErrContains: `retrieving root account from distribution seed: horizon error: "Resource Missing" - check horizon.Error.Problem for more information`,
		},
		{
			name: "returns error when GetLedgerBounds fails",
			prepareMocksFn: func() {
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(distributionAddress).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: distributionAddress}).
					Return(horizon.Account{AccountID: distributionAddress, Sequence: 1}, nil).
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
			prepareMocksFn: func() {
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(distributionAddress).
					Once()
				mChAccSigClient.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), chAccAddress).
					Return(nil, fmt.Errorf("signing remove account transaction for account")).Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: distributionAddress}).
					Return(horizon.Account{
						AccountID: distributionAddress,
						Sequence:  1,
					}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").
					Return(ledgerBounds, nil).
					Once()
			},
			wantErrContains: "signing remove account transaction for account",
		},
		{
			name:                 "returns error when fails submitting transaction to horizon",
			chAccAddressToDelete: chAccAddress,
			prepareMocksFn: func() {
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(distributionAddress).
					Twice()
				mHostAccSigClient.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), distributionAddress).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				mChAccSigClient.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), chAccAddress).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: distributionAddress}).
					Return(horizon.Account{
						AccountID: distributionAddress,
						Sequence:  1,
					}, nil).
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
				mLedgerNumberTracker.
					On("GetLedgerBounds").
					Return(ledgerBounds, nil).
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
			prepareMocksFn: func() {
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(distributionAddress).
					Twice()
				mHostAccSigClient.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), distributionAddress).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				mChAccSigClient.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), chAccAddress).
					Return(&txnbuild.Transaction{}, nil).
					Once()
				mChAccSigClient.On("Delete", ctx, chAccAddress).Return(nil).Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: distributionAddress}).
					Return(horizon.Account{
						AccountID: distributionAddress,
						Sequence:  1,
					}, nil).
					Once()
				horizonClientMock.On("SubmitTransactionWithOptions", mock.AnythingOfType("*txnbuild.Transaction"), horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true}).
					Return(horizon.Transaction{}, nil).
					Once()
				mLedgerNumberTracker.
					On("GetLedgerBounds").
					Return(ledgerBounds, nil).
					Once()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn()
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

			store.DeleteAllFromChannelAccounts(t, ctx, dbConnectionPool)
		})
	}

	horizonClientMock.AssertExpectations(t)
	privateKeyEncrypterMock.AssertExpectations(t)
}

func Test_FundDistributionAccount(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	horizonClientMock := &horizonclient.MockClient{}
	//privateKeyEncrypterMock := &utils.PrivateKeyEncrypterMock{}
	ctx := context.Background()

	hostDistributionKP := keypair.MustRandom()
	hostDistributionAddress := hostDistributionKP.Address()

	tntDistributionKP := keypair.MustRandom()
	tntDistributionAddress := tntDistributionKP.Address()
	sigService, _, _, mHostAccSigClient, mDistAccResolver := signing.NewMockSignatureService(t)

	mLedgerNumberTracker := preconditionsMocks.NewMockLedgerNumberTracker(t)

	tenantID := "ABC"
	tenantDistributionFundingAmount, err := strconv.Atoi(DefaultTenantDistributionAccountAmount)
	require.NoError(t, err)

	testCases := []struct {
		name            string
		prepareMocksFn  func()
		amountToFund    int
		wantErrContains string
	}{
		{
			name:            "amount to fund tenant distribution is not a valid amount",
			prepareMocksFn:  func() {},
			amountToFund:    0,
			wantErrContains: "invalid amount of native asset to send: 0",
		},
		{
			name:            "amount to fund tenant distribution account not in acceptable range",
			prepareMocksFn:  func() {},
			amountToFund:    1,
			wantErrContains: fmt.Sprintf("amount of native asset to send must be between %d and %d", MinTenantDistributionAccountAmount, MaxTenantDistributionAccountAmount),
		},
		{
			name: "host distribution account is same as tenant distribution account",
			prepareMocksFn: func() {
				mDistAccResolver.
					On("DistributionAccount", ctx, tenantID).Return(hostDistributionAddress, nil).Once()
				mDistAccResolver.
					On("HostDistributionAccount").Return(hostDistributionAddress).Once()
			},
			amountToFund: tenantDistributionFundingAmount,
		},
		{
			name: "returns error when HorizonClient fails getting tenant distribution AccountDetails",
			prepareMocksFn: func() {
				mDistAccResolver.
					On("DistributionAccount", ctx, tenantID).Return(tntDistributionAddress, nil).Once()
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(hostDistributionAddress).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAddress}).
					Return(horizon.Account{}, horizonclient.Error{Problem: problem.NotFound}).
					Once()
			},
			amountToFund: tenantDistributionFundingAmount,
			wantErrContains: fmt.Sprintf(
				`getting details for tenant distribution account: cannot find account on the network %s: horizon error: "Resource Missing" - check horizon.Error.Problem for more information`,
				tntDistributionAddress,
			),
		},
		{
			name: "returns error when HorizonClient fails getting host distribution AccountDetails",
			prepareMocksFn: func() {
				mDistAccResolver.
					On("DistributionAccount", ctx, tenantID).Return(tntDistributionAddress, nil).Once()
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(hostDistributionAddress).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAddress}).
					Return(horizon.Account{}, nil).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostDistributionAddress}).
					Return(horizon.Account{}, horizonclient.Error{Problem: problem.NotFound}).
					Once()
			},
			amountToFund: tenantDistributionFundingAmount,
			wantErrContains: fmt.Sprintf(
				`getting details for host distribution account: cannot find account on the network %s: horizon error: "Resource Missing" - check horizon.Error.Problem for more information`,
				hostDistributionAddress,
			),
		},
		{
			name: "returns error when failing to submit tx over Horizon",
			prepareMocksFn: func() {
				mDistAccResolver.
					On("DistributionAccount", ctx, tenantID).Return(tntDistributionAddress, nil).Once()
				mDistAccResolver.
					On("HostDistributionAccount").
					Return(hostDistributionAddress).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: tntDistributionAddress}).
					Return(horizon.Account{}, nil).
					Once()
				horizonClientMock.
					On("AccountDetail", horizonclient.AccountRequest{AccountID: hostDistributionAddress}).
					Return(horizon.Account{AccountID: hostDistributionAddress}, nil).
					Once()
				mHostAccSigClient.
					On("SignStellarTransaction", ctx, mock.AnythingOfType("*txnbuild.Transaction"), hostDistributionAddress).
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
			amountToFund: tenantDistributionFundingAmount,
			wantErrContains: fmt.Sprintf(
				`submitting payment tx from %s to %s to the Stellar network: horizon response error: StatusCode=408, Type=https://stellar.org/horizon-errors/timeout, Title=Timeout`,
				hostDistributionAddress,
				tntDistributionAddress,
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepareMocksFn != nil {
				tc.prepareMocksFn()
			}

			submitterEngine := engine.SubmitterEngine{
				HorizonClient:       horizonClientMock,
				SignatureService:    sigService,
				MaxBaseFee:          txnbuild.MinBaseFee,
				LedgerNumberTracker: mLedgerNumberTracker,
			}

			err = FundDistributionAccount(ctx, submitterEngine, tenantID, tc.amountToFund)
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
