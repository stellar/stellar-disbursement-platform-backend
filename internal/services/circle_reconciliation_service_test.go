package services

import (
	"context"
	"errors"
	"fmt"
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
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "My Wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	// Create distribution accounts
	circleDistAccountDBVault := schema.TransactionAccount{
		CircleWalletID: "circle-wallet-id",
		Type:           schema.DistributionAccountCircleDBVault,
		Status:         schema.AccountStatusActive,
	}

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	// Create payments with Circle transfer requests
	circlePendingStatus := data.CircleTransferStatusPending

	for _, objType := range []circleObjType{circleObjTypeTransfer, circleObjTypePayout} {
		t.Run(string(objType), func(t *testing.T) {
			defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
			defer data.DeleteAllCircleTransferRequests(t, ctx, dbConnectionPool)

			// prepare initial values
			circleGetterFnName := "GetTransferByID"
			if objType == circleObjTypePayout {
				circleGetterFnName = "GetPayoutByID"
			}

			circleTransferIDFn := func(suffix string) *string {
				if objType != circleObjTypeTransfer {
					return nil
				}
				return utils.StringPtr(fmt.Sprintf("circle-%s-id-%s", string(circleObjTypeTransfer), suffix))
			}
			circlePayoutIDFn := func(suffix string) *string {
				if objType != circleObjTypePayout {
					return nil
				}
				return utils.StringPtr(fmt.Sprintf("circle-%s-id-%s", string(circleObjTypePayout), suffix))
			}

			getCircleID := func(ctr data.CircleTransferRequest) string {
				if objType == circleObjTypeTransfer {
					return *ctr.CircleTransferID
				}
				return *ctr.CirclePayoutID
			}

			objConstructor := func(ctr data.CircleTransferRequest, status circle.TransferStatus) interface{} {
				if objType == circleObjTypeTransfer {
					return &circle.Transfer{ID: *ctr.CircleTransferID, Status: status}
				}

				return &circle.Payout{ID: *ctr.CirclePayoutID, Status: status}
			}

			// execute the tests
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
				CircleTransferID: circleTransferIDFn("1"),
				CirclePayoutID:   circlePayoutIDFn("1"),
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
				CircleTransferID: circleTransferIDFn("2"),
				CirclePayoutID:   circlePayoutIDFn("2"),
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
				CircleTransferID: circleTransferIDFn("3"),
				CirclePayoutID:   circlePayoutIDFn("3"),
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
				CircleTransferID: circleTransferIDFn("4"),
				CirclePayoutID:   circlePayoutIDFn("4"),
			})

			// prepare mocks
			mDistAccountResolver := sigMocks.NewMockDistributionAccountResolver(t)
			mDistAccountResolver.
				On("DistributionAccountFromContext", mock.Anything).
				Return(circleDistAccountDBVault, nil).
				Once()
			mCircleService := circle.NewMockService(t)
			mCircleService.
				On(circleGetterFnName, mock.Anything, getCircleID(*circleReq1WillThrowAnError)).
				Return(nil, errors.New("something went wrong")).
				Once().
				On(circleGetterFnName, mock.Anything, getCircleID(*circleReq2StaysPending)).
				Return(objConstructor(*circleReq2StaysPending, circle.TransferStatusPending), nil).
				Once().
				On(circleGetterFnName, mock.Anything, getCircleID(*circleReq3WillSucceed)).
				Return(objConstructor(*circleReq3WillSucceed, circle.TransferStatusComplete), nil).
				Once().
				On(circleGetterFnName, mock.Anything, getCircleID(*circleReq4WillFail)).
				Return(objConstructor(*circleReq4WillFail, circle.TransferStatusFailed), nil).
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
			assert.EqualError(t, err, fmt.Sprintf(`attempted to reconcyle 4 circle requests but failed on 1 reconciliations: [reconciling Circle transfer request: getting Circle %s by ID "circle-%s-id-1": fetching Circle %s: something went wrong]`, objType, objType, objType))

			// assert logs
			entries := getEntries()
			var messages []string
			for _, entry := range entries {
				messages = append(messages, entry.Message)
			}
			assert.Contains(t, messages, fmt.Sprintf(`[tenant=test-tenant] Reconciled Circle %s request "circle-%s-id-3" with status "complete"`, objType, objType))
			assert.Contains(t, messages, fmt.Sprintf(`[tenant=test-tenant] Reconciled Circle %s request "circle-%s-id-4" with status "failed"`, objType, objType))

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
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "My Wallet", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	// Create payments with Circle transfer requests
	circlePendingStatus := data.CircleTransferStatusPending

	for _, objType := range []circleObjType{circleObjTypeTransfer, circleObjTypePayout} {
		t.Run(string(objType), func(t *testing.T) {
			defer data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
			defer data.DeleteAllCircleTransferRequests(t, ctx, dbConnectionPool)

			// prepare initial values
			circleGetterFnName := "GetTransferByID"
			getterID := "circle-transfer-id"
			if objType == circleObjTypePayout {
				circleGetterFnName = "GetPayoutByID"
				getterID = "circle-payout-id"
			}

			var circleTransferID, circlePayoutID *string
			if objType == circleObjTypePayout {
				circlePayoutID = &getterID
			} else if objType == circleObjTypeTransfer {
				circleTransferID = &getterID
			}

			objConstructor := func(ctr data.CircleTransferRequest, status circle.TransferStatus) interface{} {
				if objType == circleObjTypeTransfer {
					return &circle.Transfer{ID: *ctr.CircleTransferID, Status: status}
				}

				return &circle.Payout{ID: *ctr.CirclePayoutID, Status: status}
			}

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
				CircleTransferID: circleTransferID,
				CirclePayoutID:   circlePayoutID,
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
							On(circleGetterFnName, mock.Anything, getterID).
							Return(nil, &circle.APIError{StatusCode: http.StatusUnauthorized}).
							Once()
					},
					wantErrorContains: []string{"getting Circle", string(objType), "by ID", "APIError", "StatusCode=401"},
				},
				{
					name: "403 should be logged and an error should be returned",
					setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
						mCircleService.
							On(circleGetterFnName, mock.Anything, getterID).
							Return(nil, &circle.APIError{StatusCode: http.StatusForbidden}).
							Once()
					},
					wantErrorContains: []string{"getting Circle", string(objType), "by ID", "APIError", "StatusCode=403"},
				},
				{
					name: "404 should be logged and an error should be returned",
					setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
						mCircleService.
							On(circleGetterFnName, mock.Anything, getterID).
							Return(nil, &circle.APIError{StatusCode: http.StatusNotFound}).
							Once()
					},
					wantErrorContains: []string{"getting Circle", string(objType), "by ID", "APIError", "StatusCode=404"},
				},
				{
					name: "429 should be logged and an error should be returned",
					setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
						mCircleService.
							On(circleGetterFnName, mock.Anything, getterID).
							Return(nil, &circle.APIError{StatusCode: http.StatusTooManyRequests}).
							Once()
					},
					wantErrorContains: []string{"getting Circle", string(objType), "by ID", "APIError", "StatusCode=429"},
				},
				{
					name: "5xx should be logged and an error should be returned",
					setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
						mCircleService.
							On(circleGetterFnName, mock.Anything, getterID).
							Return(nil, &circle.APIError{StatusCode: http.StatusInternalServerError}).
							Once()
					},
					wantErrorContains: []string{"getting Circle", string(objType), "by ID", "APIError", "StatusCode=500"},
				},
				{
					name: "non-API error should be logged and an error should be returned",
					setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
						mCircleService.
							On(circleGetterFnName, mock.Anything, getterID).
							Return(nil, errors.New("test-error")).
							Once()
					},
					wantErrorContains: []string{"getting Circle", string(objType), "by ID", "test-error"},
				},
				{
					name: "400 should increment the sync attempts and an error should be returned",
					setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
						mCircleService.
							On(circleGetterFnName, mock.Anything, getterID).
							Return(nil, &circle.APIError{Message: "foo bar", StatusCode: http.StatusBadRequest}).
							Once()
					},
					wantErrorContains:           []string{"getting Circle", string(objType), "by ID", "APIError", "StatusCode=400"},
					shouldIncrementSyncAttempts: true,
				},
				{
					name: "200 should increment the sync attempts and return nil",
					setupMocksAndDBFn: func(t *testing.T, mCircleService *circle.MockService) {
						mCircleService.
							On(circleGetterFnName, mock.Anything, getterID).
							Return(objConstructor(*circleRequest, circle.TransferStatusComplete), nil).
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
					err = svc.reconcilePaymentRequest(ctx, dbTx, tnt, circleRequest)

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
						assert.NotNil(t, circleReqFromDB.ResponseBody)
					} else {
						assert.Equal(t, circleRequest.SyncAttempts, circleReqFromDB.SyncAttempts)
						assert.Nil(t, circleReqFromDB.LastSyncAttemptAt)
						assert.Nil(t, circleReqFromDB.ResponseBody)
					}
				})
			}
		})
	}
}
