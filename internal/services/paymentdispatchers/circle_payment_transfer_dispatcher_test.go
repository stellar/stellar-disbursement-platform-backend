package paymentdispatchers

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_CirclePaymentTransferDispatcher_DispatchPayments_failure(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	tenantID := "tenant-id"
	ctx := context.Background()
	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	circleWalletID := "22322112"

	// Disbursement
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})
	// Receiver
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	// Receiver Wallets
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)
	payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *disbursement.Asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})

	testCases := []struct {
		name              string
		paymentToDispatch *data.Payment
		wantErr           error
		fnSetup           func(*testing.T, *circle.MockService)
		fnAssert          func(*testing.T, data.Payment)
	}{
		{
			name:              "failure validating payment ready for sending",
			paymentToDispatch: &data.Payment{ID: "123"},
			wantErr:           fmt.Errorf("payment with ID 123 does not exist"),
		},
		{
			name:              "payment marked as failed when posting circle transfer fails",
			paymentToDispatch: payment,
			wantErr:           nil,
			fnSetup: func(t *testing.T, m *circle.MockService) {
				transferRequest, setupErr := models.CircleTransferRequests.Insert(ctx, dbConnectionPool, payment.ID)
				require.NoError(t, setupErr)

				m.On("SendTransfer", ctx, circle.PaymentRequest{
					APIType:                   circle.APITypeTransfers,
					SourceWalletID:            circleWalletID,
					DestinationStellarAddress: payment.ReceiverWallet.StellarAddress,
					DestinationStellarMemo:    payment.ReceiverWallet.StellarMemo,
					Amount:                    payment.Amount,
					StellarAssetCode:          payment.Asset.Code,
					IdempotencyKey:            transferRequest.IdempotencyKey,
				}).
					Return(nil, fmt.Errorf("posting transfer to Circle")).
					Once()
			},
			fnAssert: func(t *testing.T, p data.Payment) {
				assert.Equal(t, data.FailedPaymentStatus, p.Status)
			},
		},
		{
			name:              "error updating circle transfer request",
			paymentToDispatch: payment,
			wantErr:           fmt.Errorf("updating circle transfer request: transfer cannot be nil"),
			fnSetup: func(t *testing.T, m *circle.MockService) {
				m.On("SendTransfer", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(nil, nil).
					Once()
			},
		},
		{
			name:              "error updating payment status for completed request",
			paymentToDispatch: payment,
			wantErr:           fmt.Errorf("invalid input value for enum circle_transfer_status"),
			fnSetup: func(t *testing.T, m *circle.MockService) {
				m.On("SendTransfer", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(&circle.Transfer{
						ID:     "transfer_id",
						Status: "wrong-status",
					}, nil).
					Once()
			},
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

			mCircleService := circle.NewMockService(t)
			mDistAccountResolver := &mocks.MockDistributionAccountResolver{}
			mDistAccountResolver.
				On("DistributionAccountFromContext", ctx).
				Return(schema.TransactionAccount{
					Type:           schema.DistributionAccountCircleDBVault,
					CircleWalletID: circleWalletID,
					Status:         schema.AccountStatusActive,
				}, nil).Maybe()

			dispatcher := NewCirclePaymentTransferDispatcher(models, mCircleService, mDistAccountResolver)

			if tc.fnSetup != nil {
				tc.fnSetup(t, mCircleService)
			}

			err = dispatcher.DispatchPayments(ctx, dbTx, tenantID, []*data.Payment{tc.paymentToDispatch})
			if tc.wantErr != nil {
				assert.ErrorContains(t, err, tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}

			if tc.fnAssert != nil {
				paymentFromDB, err := models.Payment.Get(ctx, tc.paymentToDispatch.ID, dbTx)
				require.NoError(t, err)
				tc.fnAssert(t, *paymentFromDB)
			}
		})
	}
}

func Test_CirclePaymentTransferDispatcher_DispatchPayments_success(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	tenantID := "tenant-id"
	tnt := schema.Tenant{
		ID:      tenantID,
		BaseURL: utils.Ptr("https://example.com"),
	}

	ctx := context.Background()
	ctx = sdpcontext.SetTenantInContext(ctx, &tnt)
	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	circleWalletID := "22322112"
	circleTransferID := uuid.NewString()

	// Wallets
	walletA := data.CreateWalletFixture(t, ctx, dbConnectionPool, "walletA", "https://www.a.com", "www.a.com", "a://")
	walletB := data.CreateWalletFixture(t, ctx, dbConnectionPool, "walletB", "https://www.b.com", "www.b.com", "b://")
	// Disbursement
	disbursementA := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{Wallet: walletA})
	disbursementB := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{Wallet: walletB})
	// Receiver
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	// Receiver Wallets
	rwWithMemo := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursementA.Wallet.ID, data.RegisteredReceiversWalletStatus)
	paymentWithMemo := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwWithMemo,
		Disbursement:   disbursementA,
		Asset:          *disbursementA.Asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})

	rwWithoutMemo := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, disbursementB.Wallet.ID, data.RegisteredReceiversWalletStatus)
	outerErr = models.ReceiverWallet.Update(ctx, rwWithoutMemo.ID, data.ReceiverWalletUpdate{
		StellarMemo:     utils.Ptr(""),
		StellarMemoType: utils.Ptr(schema.MemoType("")),
	}, dbConnectionPool)
	require.NoError(t, outerErr)
	paymentWithoutMemo := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwWithoutMemo,
		Disbursement:   disbursementB,
		Asset:          *disbursementB.Asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})

	testCases := []struct {
		name                 string
		IsMemoTracingEnabled bool
		paymentToDispatch    *data.Payment
		fnAssertMemo         func(t *testing.T, p data.Payment, pReq circle.PaymentRequest)
	}{
		{
			name:                 "posting a Circle Transfer with ReceiverWallet memo",
			IsMemoTracingEnabled: false,
			paymentToDispatch:    paymentWithMemo,
			fnAssertMemo: func(t *testing.T, p data.Payment, pReq circle.PaymentRequest) {
				assert.Equal(t, p.ReceiverWallet.StellarMemo, pReq.DestinationStellarMemo)
			},
		},
		{
			name:                 "posting a Circle Transfer without ReceiverWallet nor Organization memo",
			IsMemoTracingEnabled: false,
			paymentToDispatch:    paymentWithoutMemo,
			fnAssertMemo: func(t *testing.T, p data.Payment, pReq circle.PaymentRequest) {
				assert.Empty(t, pReq.DestinationStellarMemo)
			},
		},
		{
			name:                 "posting a Circle Transfer with Organization memo enabled",
			IsMemoTracingEnabled: true,
			paymentToDispatch:    paymentWithoutMemo,
			fnAssertMemo: func(t *testing.T, p data.Payment, pReq circle.PaymentRequest) {
				assert.Equal(t, tenant.GenerateHashFromBaseURL(*tnt.BaseURL), pReq.DestinationStellarMemo)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := models.Organizations.Update(ctx, &data.OrganizationUpdate{IsMemoTracingEnabled: utils.Ptr(tc.IsMemoTracingEnabled)})
			require.NoError(t, err)

			dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() {
				require.NoError(t, dbTx.Rollback())
				data.DeleteAllCircleTransferRequests(t, ctx, dbConnectionPool)
			}()

			mCircleService := circle.NewMockService(t)
			mDistAccountResolver := &mocks.MockDistributionAccountResolver{}
			mDistAccountResolver.
				On("DistributionAccountFromContext", ctx).
				Return(schema.TransactionAccount{
					Type:           schema.DistributionAccountCircleDBVault,
					CircleWalletID: circleWalletID,
					Status:         schema.AccountStatusActive,
				}, nil).Maybe()

			dispatcher := NewCirclePaymentTransferDispatcher(models, mCircleService, mDistAccountResolver)

			p := tc.paymentToDispatch
			mCircleService.
				On("SendTransfer", ctx, mock.AnythingOfType("circle.PaymentRequest")).
				Run(func(args mock.Arguments) {
					pReq, ok := args.Get(1).(circle.PaymentRequest)
					require.True(t, ok)

					assert.Equal(t, circle.APITypeTransfers, pReq.APIType)
					assert.Equal(t, circleWalletID, pReq.SourceWalletID)
					assert.Equal(t, p.Amount, pReq.Amount)
					assert.Equal(t, p.Asset.Code, pReq.StellarAssetCode)
					assert.Equal(t, p.ReceiverWallet.StellarAddress, pReq.DestinationStellarAddress)

					tc.fnAssertMemo(t, *p, pReq)
				}).
				Return(&circle.Transfer{
					ID:     circleTransferID,
					Status: circle.TransferStatusPending,
					Source: circle.TransferAccount{
						ID:   circleWalletID,
						Type: circle.TransferAccountTypeWallet,
					},
					Destination: circle.TransferAccount{
						Address:    p.ReceiverWallet.StellarAddress,
						AddressTag: p.ReceiverWallet.StellarMemo,
						Chain:      circle.StellarChainCode,
					},
					Amount: circle.Balance{
						Amount:   p.Amount,
						Currency: "USD",
					},
				}, nil).
				Once()

			err = dispatcher.DispatchPayments(ctx, dbTx, tenantID, []*data.Payment{p})
			assert.NoError(t, err)

			// Payment should be marked as pending
			paymentFromDB, err := models.Payment.Get(ctx, p.ID, dbTx)
			require.NoError(t, err)
			assert.Equal(t, data.PendingPaymentStatus, paymentFromDB.Status)

			// Transfer request is still not updated for the main connection pool
			queryParams := data.QueryParams{Filters: map[data.FilterKey]interface{}{data.FilterKeyPaymentID: p.ID}}
			transferRequest, err := models.CircleTransferRequests.Get(ctx, dbConnectionPool, queryParams)
			require.NoError(t, err)
			assert.Nil(t, transferRequest.CircleTransferID)
			assert.Nil(t, transferRequest.SourceWalletID)

			// Transfer request is updated for the transaction
			transferRequest, err = models.CircleTransferRequests.Get(ctx, dbTx, queryParams)
			require.NoError(t, err)
			assert.Equal(t, circleTransferID, *transferRequest.CircleTransferID)
			assert.Equal(t, circleWalletID, *transferRequest.SourceWalletID)
			assert.Equal(t, data.CircleTransferStatusPending, *transferRequest.Status)
		})
	}
}

func Test_CirclePaymentTransferDispatcher_SupportedPlatform(t *testing.T) {
	dispatcher := CirclePaymentTransferDispatcher{}
	assert.Equal(t, schema.CirclePlatform, dispatcher.SupportedPlatform())
}
