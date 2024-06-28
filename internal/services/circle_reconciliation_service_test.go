package services

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_NewCircleReconciliationService_Reconcile_failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	tnt := &tenant.Tenant{ID: "95e788b6-c80e-4975-9d12-141001fe6e44", Name: "test-tenant"}

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	// Create distribution accounts
	stellarDistAccountEnv := schema.NewStellarEnvTransactionAccount("GAAHIL6ZW4QFNLCKALZ3YOIWPP4TXQ7B7J5IU7RLNVGQAV6GFDZHLDTA")
	innactiveCircleDistAccountDBVault := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusPendingUserActivation,
	}
	circleDistAccountDBVault := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}

	testCases := []struct {
		name              string
		tenant            *tenant.Tenant
		setupMocksAndDBFn func(t *testing.T, mDistAccountResolver *sigMocks.MockDistributionAccountResolver, mCircleService *circle.MockService)
		wantErrorContains string
		assertLogsFn      func(entries []logrus.Entry)
	}{
		{
			name:              "returns error when getting tenant from context fails",
			wantErrorContains: "getting tenant from context",
		},
		{
			name:   "returns error when getting distribution account from context fails",
			tenant: tnt,
			setupMocksAndDBFn: func(t *testing.T, mDistAccountResolver *sigMocks.MockDistributionAccountResolver, _ *circle.MockService) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(schema.TransactionAccount{}, assert.AnError).
					Once()
			},
			wantErrorContains: "getting distribution account from context",
		},
		{
			name:   "skips reconciliation when distribution account is not of type CIRCLE",
			tenant: tnt,
			setupMocksAndDBFn: func(t *testing.T, mDistAccountResolver *sigMocks.MockDistributionAccountResolver, mCircleService *circle.MockService) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(stellarDistAccountEnv, nil).
					Once()
			},
			assertLogsFn: func(entries []logrus.Entry) {
				assert.Equal(t, "Distribution account for tenant \"test-tenant\" is not of type \"CIRCLE\", skipping reconciliation...", entries[0].Message)
			},
		},
		{
			name:   "skips reconciliation when distribution account is CIRCLE but it's not ACTIVE",
			tenant: tnt,
			setupMocksAndDBFn: func(t *testing.T, mDistAccountResolver *sigMocks.MockDistributionAccountResolver, mCircleService *circle.MockService) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(innactiveCircleDistAccountDBVault, nil).
					Once()
			},
			assertLogsFn: func(entries []logrus.Entry) {
				assert.Equal(t, "Distribution account for tenant \"test-tenant\" is not \"ACTIVE\", skipping reconciliation...", entries[0].Message)
			},
		},
		{
			name:   "skips reconciliation when there are no pending Circle transfer requests",
			tenant: tnt,
			setupMocksAndDBFn: func(t *testing.T, mDistAccountResolver *sigMocks.MockDistributionAccountResolver, mCircleService *circle.MockService) {
				mDistAccountResolver.
					On("DistributionAccountFromContext", mock.Anything).
					Return(circleDistAccountDBVault, nil).
					Once()
			},
			assertLogsFn: func(entries []logrus.Entry) {
				assert.Equal(t, "Found 0 pending Circle transfer requests in tenant \"test-tenant\"", entries[0].Message)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// prepare mocks
			mDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
			mCircleService := circle.NewMockService(t)
			svc := CircleReconciliationService{
				Models:              models,
				CircleService:       mCircleService,
				DistAccountResolver: mDistAccountResolver,
			}
			if tc.setupMocksAndDBFn != nil {
				tc.setupMocksAndDBFn(t, mDistAccountResolver, mCircleService)
			}

			// inject tenant in context if configured
			updatedCtx := ctx
			if tc.tenant != nil {
				updatedCtx = tenant.SaveTenantInContext(ctx, tc.tenant)
			}

			// run test
			getEntries := log.DefaultLogger.StartTest(logrus.DebugLevel)
			err := svc.Reconcile(updatedCtx)

			// asserttions
			if tc.wantErrorContains != "" {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrorContains)
			} else {
				assert.NoError(t, err)
			}
			entries := getEntries()
			if tc.assertLogsFn != nil {
				tc.assertLogsFn(entries)
			}
		})
	}
}

func Test_NewCircleReconciliationService_Reconcile_success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tnt := &tenant.Tenant{ID: "95e788b6-c80e-4975-9d12-141001fe6e44", Name: "test-tenant"}
	ctx := tenant.SaveTenantInContext(context.Background(), tnt)

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, assets.EURCAssetCode, assets.EURCAssetTestnet.Issuer)
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "My Wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	// Create distribution accounts
	circleDistAccountDBVault := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}

	// database cleanup
	defer data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
	defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	defer data.DeleteAllReceiverVerificationFixtures(t, ctx, dbConnectionPool)
	defer data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	defer data.DeleteAllCircleTransferRequestsFixtures(t, ctx, dbConnectionPool)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement",
		Status:  data.StartedDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	// Create payments with Circle transfer requests
	p1StaysPending := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	circlePendingStatus := data.CircleTransferStatusPending
	circleReq1StaysPending := data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		PaymentID:        p1StaysPending.ID,
		Status:           &circlePendingStatus,
		CircleTransferID: utils.StringPtr("circle-transfer-id-1"),
	})

	p2WillSucceed := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	circleReq2WillSucceed := data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		PaymentID:        p2WillSucceed.ID,
		Status:           &circlePendingStatus,
		CircleTransferID: utils.StringPtr("circle-transfer-id-2"),
	})

	p3WillFail := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	circleReq3WillFail := data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		PaymentID:        p3WillFail.ID,
		Status:           &circlePendingStatus,
		CircleTransferID: utils.StringPtr("circle-transfer-id-3"),
	})

	// prepare mocks
	mDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistAccountResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(circleDistAccountDBVault, nil).
		Once()
	mCircleService := circle.NewMockService(t)
	mCircleService.
		On("GetTransferByID", mock.Anything, *circleReq1StaysPending.CircleTransferID).
		Return(&circle.Transfer{
			ID:     *circleReq1StaysPending.CircleTransferID,
			Status: circle.TransferStatusPending,
		}, nil).
		Once().
		On("GetTransferByID", mock.Anything, *circleReq2WillSucceed.CircleTransferID).
		Return(&circle.Transfer{
			ID:     *circleReq2WillSucceed.CircleTransferID,
			Status: circle.TransferStatusComplete,
		}, nil).
		Once().
		On("GetTransferByID", mock.Anything, *circleReq3WillFail.CircleTransferID).
		Return(&circle.Transfer{
			ID:     *circleReq3WillFail.CircleTransferID,
			Status: circle.TransferStatusFailed,
		}, nil).
		Once()

	// run test
	getEntries := log.DefaultLogger.StartTest(logrus.DebugLevel)
	svc := CircleReconciliationService{
		Models:              models,
		CircleService:       mCircleService,
		DistAccountResolver: mDistAccountResolver,
	}
	err = svc.Reconcile(ctx)
	assert.NoError(t, err)

	// assert logs
	entries := getEntries()
	var messages []string
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	assert.Contains(t, messages, `[tenant=test-tenant] Reconciled Circle transfer request "circle-transfer-id-2" with status "complete"`)
	assert.Contains(t, messages, `[tenant=test-tenant] Reconciled Circle transfer request "circle-transfer-id-3" with status "failed"`)

	// assert results
	updatedCircleRequestAndPayment := func(paymentID string) (*data.CircleTransferRequest, *data.Payment) {
		updatedCircleRequest, err := models.CircleTransferRequests.Get(ctx, dbConnectionPool, data.QueryParams{Filters: map[data.FilterKey]interface{}{data.FilterKeyPaymentID: paymentID}})
		require.NoError(t, err)

		updatedPayment, err := models.Payment.Get(ctx, paymentID, dbConnectionPool)
		require.NoError(t, err)

		return updatedCircleRequest, updatedPayment
	}
	// p1StaysPending
	updatedCircleReq1, updatedPayment1 := updatedCircleRequestAndPayment(p1StaysPending.ID)
	assert.Equal(t, data.CircleTransferStatusPending, *updatedCircleReq1.Status)
	assert.Equal(t, data.PendingPaymentStatus, updatedPayment1.Status)
	// p2WillSucceed
	updatedCircleReq2, updatedPayment2 := updatedCircleRequestAndPayment(p2WillSucceed.ID)
	assert.Equal(t, data.CircleTransferStatusSuccess, *updatedCircleReq2.Status)
	assert.Equal(t, data.SuccessPaymentStatus, updatedPayment2.Status)
	// p3WillFail
	updatedCircleReq3, updatedPayment3 := updatedCircleRequestAndPayment(p3WillFail.ID)
	assert.Equal(t, data.CircleTransferStatusFailed, *updatedCircleReq3.Status)
	assert.Equal(t, data.FailedPaymentStatus, updatedPayment3.Status)
}
