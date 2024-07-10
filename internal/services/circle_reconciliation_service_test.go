package services

import (
	"context"
	"errors"
	"net/http"
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

func Test_NewCircleReconciliationService_Reconcile_partialSuccess(t *testing.T) {
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
	circlePendingStatus := data.CircleTransferStatusPending

	p1WillThrowAnError := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	circleReq1WillThrowAnError := data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		PaymentID:        p1WillThrowAnError.ID,
		Status:           &circlePendingStatus,
		CircleTransferID: utils.StringPtr("circle-transfer-id-1"),
	})

	p2StaysPending := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	circleReq2StaysPending := data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		PaymentID:        p2StaysPending.ID,
		Status:           &circlePendingStatus,
		CircleTransferID: utils.StringPtr("circle-transfer-id-2"),
	})

	p3WillSucceed := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	circleReq3WillSucceed := data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		PaymentID:        p3WillSucceed.ID,
		Status:           &circlePendingStatus,
		CircleTransferID: utils.StringPtr("circle-transfer-id-3"),
	})

	p4WillFail := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	circleReq4WillFail := data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		PaymentID:        p4WillFail.ID,
		Status:           &circlePendingStatus,
		CircleTransferID: utils.StringPtr("circle-transfer-id-4"),
	})

	// prepare mocks
	mDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
	mDistAccountResolver.
		On("DistributionAccountFromContext", mock.Anything).
		Return(circleDistAccountDBVault, nil).
		Once()
	mCircleService := circle.NewMockService(t)
	mCircleService.
		On("GetTransferByID", mock.Anything, *circleReq1WillThrowAnError.CircleTransferID).
		Return(nil, errors.New("something went wrong")).
		Once().
		On("GetTransferByID", mock.Anything, *circleReq2StaysPending.CircleTransferID).
		Return(&circle.Transfer{
			ID:     *circleReq2StaysPending.CircleTransferID,
			Status: circle.TransferStatusPending,
		}, nil).
		Once().
		On("GetTransferByID", mock.Anything, *circleReq3WillSucceed.CircleTransferID).
		Return(&circle.Transfer{
			ID:     *circleReq3WillSucceed.CircleTransferID,
			Status: circle.TransferStatusComplete,
		}, nil).
		Once().
		On("GetTransferByID", mock.Anything, *circleReq4WillFail.CircleTransferID).
		Return(&circle.Transfer{
			ID:     *circleReq4WillFail.CircleTransferID,
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
	assert.Error(t, err)
	assert.EqualError(t, err, "attempted to reconcyle 4 circle requests but failed on 1 reconciliations: [reconciling Circle transfer request: getting Circle transfer by ID \"circle-transfer-id-1\": something went wrong]")

	// assert logs
	entries := getEntries()
	var messages []string
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	assert.Contains(t, messages, `[tenant=test-tenant] Reconciled Circle transfer request "circle-transfer-id-3" with status "complete"`)
	assert.Contains(t, messages, `[tenant=test-tenant] Reconciled Circle transfer request "circle-transfer-id-4" with status "failed"`)

	// assert results
	getPaymentAndCircleRequestFromDB := func(paymentID string) (*data.CircleTransferRequest, *data.Payment) {
		updatedCircleRequest, err := models.CircleTransferRequests.Get(ctx, dbConnectionPool, data.QueryParams{Filters: map[data.FilterKey]interface{}{data.FilterKeyPaymentID: paymentID}})
		require.NoError(t, err)

		updatedPayment, err := models.Payment.Get(ctx, paymentID, dbConnectionPool)
		require.NoError(t, err)

		return updatedCircleRequest, updatedPayment
	}
	// p1WillThrowAnError
	updatedCircleReq1, updatedPayment1 := getPaymentAndCircleRequestFromDB(p1WillThrowAnError.ID)
	assert.Equal(t, data.CircleTransferStatusPending, *updatedCircleReq1.Status)
	assert.Equal(t, data.PendingPaymentStatus, updatedPayment1.Status)
	// p2StaysPending
	updatedCircleReq2, updatedPayment2 := getPaymentAndCircleRequestFromDB(p2StaysPending.ID)
	assert.Equal(t, data.CircleTransferStatusPending, *updatedCircleReq2.Status)
	assert.Equal(t, data.PendingPaymentStatus, updatedPayment2.Status)
	// p3WillSucceed
	updatedCircleReq3, updatedPayment3 := getPaymentAndCircleRequestFromDB(p3WillSucceed.ID)
	assert.Equal(t, data.CircleTransferStatusSuccess, *updatedCircleReq3.Status)
	assert.Equal(t, data.SuccessPaymentStatus, updatedPayment3.Status)
	// p4WillFail
	updatedCircleReq4, updatedPayment4 := getPaymentAndCircleRequestFromDB(p4WillFail.ID)
	assert.Equal(t, data.CircleTransferStatusFailed, *updatedCircleReq4.Status)
	assert.Equal(t, data.FailedPaymentStatus, updatedPayment4.Status)
}

func Test_shouldIncrementSyncAttempts(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "false when error is not a Circle API error",
			err:      errors.New("test-error"),
			expected: false,
		},
		{
			name:     "false when error is a Circle API error but status code is not 400",
			err:      &circle.APIError{StatusCode: http.StatusUnauthorized},
			expected: false,
		},
		{
			name:     "true when error is a Circle API error and status code is 400",
			err:      &circle.APIError{StatusCode: http.StatusBadRequest},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, shouldIncrementSyncAttempts(tc.err))
		})
	}
}

func Test_NewCircleReconciliationService_reconcileTransferRequest(t *testing.T) {
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
	circlePendingStatus := data.CircleTransferStatusPending

	payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.PendingPaymentStatus,
	})
	circleRequest := data.CreateCircleTransferRequestFixture(t, ctx, dbConnectionPool, data.CircleTransferRequest{
		PaymentID:        payment.ID,
		Status:           &circlePendingStatus,
		CircleTransferID: utils.StringPtr("circle-transfer-id"),
	})

	testCases := []struct {
		name                        string
		setupMocksAndDBFn           func(t *testing.T, mCircleService *circle.MockService)
		wantErrorContains           []string
		shouldIncrementSyncAttempts bool
		assertLogsFn                func(entries []logrus.Entry)
	}{
		{
			name: "401 should be logged and an error should be returned",
			setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("GetTransferByID", mock.Anything, "circle-transfer-id").
					Return(nil, &circle.APIError{StatusCode: http.StatusUnauthorized}).
					Once()
			},
			wantErrorContains: []string{"getting Circle transfer by ID", "APIError", "StatusCode=401"},
		},
		{
			name: "403 should be logged and an error should be returned",
			setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("GetTransferByID", mock.Anything, "circle-transfer-id").
					Return(nil, &circle.APIError{StatusCode: http.StatusForbidden}).
					Once()
			},
			wantErrorContains: []string{"getting Circle transfer by ID", "APIError", "StatusCode=403"},
		},
		{
			name: "404 should be logged and an error should be returned",
			setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("GetTransferByID", mock.Anything, "circle-transfer-id").
					Return(nil, &circle.APIError{StatusCode: http.StatusNotFound}).
					Once()
			},
			wantErrorContains: []string{"getting Circle transfer by ID", "APIError", "StatusCode=404"},
		},
		{
			name: "429 should be logged and an error should be returned",
			setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("GetTransferByID", mock.Anything, "circle-transfer-id").
					Return(nil, &circle.APIError{StatusCode: http.StatusTooManyRequests}).
					Once()
			},
			wantErrorContains: []string{"getting Circle transfer by ID", "APIError", "StatusCode=429"},
		},
		{
			name: "5xx should be logged and an error should be returned",
			setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("GetTransferByID", mock.Anything, "circle-transfer-id").
					Return(nil, &circle.APIError{StatusCode: http.StatusInternalServerError}).
					Once()
			},
			wantErrorContains: []string{"getting Circle transfer by ID", "APIError", "StatusCode=500"},
		},
		{
			name: "non-API error should be logged and an error should be returned",
			setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("GetTransferByID", mock.Anything, "circle-transfer-id").
					Return(nil, errors.New("test-error")).
					Once()
			},
			wantErrorContains: []string{"getting Circle transfer by ID", "test-error"},
		},
		{
			name: "400 should increment the sync attempts and an error should be returned",
			setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("GetTransferByID", mock.Anything, "circle-transfer-id").
					Return(nil, &circle.APIError{StatusCode: http.StatusBadRequest}).
					Once()
			},
			wantErrorContains:           []string{"getting Circle transfer by ID", "APIError", "StatusCode=400"},
			shouldIncrementSyncAttempts: true,
		},
		{
			name: "200 should increment the sync attempts and return nil",
			setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
				mCircleService.
					On("GetTransferByID", mock.Anything, "circle-transfer-id").
					Return(&circle.Transfer{
						ID:     *circleRequest.CircleTransferID,
						Status: circle.TransferStatusComplete,
					}, nil).
					Once()
			},
			shouldIncrementSyncAttempts: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				err = dbTx.Rollback()
				require.NoError(t, err)
			}()

			// prepare mocks
			mCircleService := circle.NewMockService(t)
			tc.setupMocksAndDBFn(t, mCircleService)
			svc := CircleReconciliationService{
				Models:        models,
				CircleService: mCircleService,
			}
			err = svc.reconcileTransferRequest(ctx, dbTx, tnt, circleRequest)

			// get the updated CircleRequestTransfer and Payment from the DB
			circleReqFromDB, dbErr := models.CircleTransferRequests.Get(ctx, dbTx, data.QueryParams{
				Filters: map[data.FilterKey]interface{}{
					data.FilterKeyPaymentID: circleRequest.PaymentID,
				},
			})
			require.NoError(t, dbErr)
			paymentFromDB, dbErr := models.Payment.Get(ctx, circleRequest.PaymentID, dbTx)
			require.NoError(t, dbErr)

			if len(tc.wantErrorContains) != 0 {
				require.Error(t, err)
				for _, wantErrorContains := range tc.wantErrorContains {
					assert.ErrorContains(t, err, wantErrorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, data.CircleTransferStatusSuccess, *circleReqFromDB.Status)
				assert.Equal(t, data.SuccessPaymentStatus, paymentFromDB.Status)
			}

			if tc.shouldIncrementSyncAttempts {
				assert.Equal(t, circleRequest.SyncAttempts+1, circleReqFromDB.SyncAttempts)
				assert.NotNil(t, circleReqFromDB.LastSyncAttemptAt)
			} else {
				assert.Equal(t, circleRequest.SyncAttempts, circleReqFromDB.SyncAttempts)
				assert.Nil(t, circleReqFromDB.LastSyncAttemptAt)
			}
		})
	}
}
