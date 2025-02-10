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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_CirclePaymentTransferDispatcher_DispatchPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	circleWalletID := "22322112"
	circleTransferID := uuid.NewString()

	tenantID := "tenant-id"

	// Disbursement
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{})

	// Receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	// Receiver Wallets
	rw1Registered := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, disbursement.Wallet.ID, data.RegisteredReceiversWalletStatus)

	// Payments
	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw1Registered,
		Disbursement:   disbursement,
		Asset:          *disbursement.Asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})

	tests := []struct {
		name               string
		paymentsToDispatch []*data.Payment
		wantErr            error
		fnSetup            func(*testing.T, *circle.MockService)
		fnAsserts          func(*testing.T, db.SQLExecuter)
	}{
		{
			name: "failure validating payment ready for sending",
			paymentsToDispatch: []*data.Payment{
				{ID: "123"},
			},
			wantErr: fmt.Errorf("payment with ID 123 does not exist"),
		},
		{
			name:               "payment marked as failed when posting circle transfer fails",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            nil,
			fnSetup: func(t *testing.T, m *circle.MockService) {
				transferRequest, setupErr := models.CircleTransferRequests.Insert(ctx, payment1.ID)
				require.NoError(t, setupErr)

				m.On("SendTransfer", ctx, circle.PaymentRequest{
					APIType:                   circle.APITypeTransfers,
					SourceWalletID:            circleWalletID,
					DestinationStellarAddress: payment1.ReceiverWallet.StellarAddress,
					DestinationStellarMemo:    payment1.ReceiverWallet.StellarMemo,
					Amount:                    payment1.Amount,
					StellarAssetCode:          payment1.Asset.Code,
					IdempotencyKey:            transferRequest.IdempotencyKey,
				}).
					Return(nil, fmt.Errorf("error posting transfer to Circle")).
					Once()
			},
			fnAsserts: func(t *testing.T, sqlExecuter db.SQLExecuter) {
				// Payment should be marked as failed
				payment, assertErr := models.Payment.Get(ctx, payment1.ID, sqlExecuter)
				require.NoError(t, assertErr)
				assert.Equal(t, data.FailedPaymentStatus, payment.Status)
			},
		},
		{
			name:               "error updating circle transfer request",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            fmt.Errorf("updating circle transfer request: transfer cannot be nil"),
			fnSetup: func(t *testing.T, m *circle.MockService) {
				m.On("SendTransfer", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(nil, nil).
					Once()
			},
		},
		{
			name:               "error updating payment status for completed request",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            fmt.Errorf("invalid input value for enum circle_transfer_status"),
			fnSetup: func(t *testing.T, m *circle.MockService) {
				m.On("SendTransfer", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(&circle.Transfer{
						ID:     "transfer_id",
						Status: "wrong-status",
					}, nil).
					Once()
			},
		},
		{
			name:               "success posting transfer to Circle",
			paymentsToDispatch: []*data.Payment{payment1},
			wantErr:            nil,
			fnSetup: func(t *testing.T, m *circle.MockService) {
				m.On("SendTransfer", ctx, mock.AnythingOfType("circle.PaymentRequest")).
					Return(&circle.Transfer{
						ID:     circleTransferID,
						Status: circle.TransferStatusPending,
						Amount: circle.Balance{
							Amount:   payment1.Amount,
							Currency: "USD",
						},
					}, nil).
					Once()
			},
			fnAsserts: func(t *testing.T, sqlExecuter db.SQLExecuter) {
				// Payment should be marked as pending
				payment, assertErr := models.Payment.Get(ctx, payment1.ID, sqlExecuter)
				require.NoError(t, assertErr)
				assert.Equal(t, data.PendingPaymentStatus, payment.Status)

				// Transfer request is still not updated for the main connection pool
				var transferRequest data.CircleTransferRequest
				assertErr = dbConnectionPool.GetContext(ctx, &transferRequest, "SELECT * FROM circle_transfer_requests WHERE payment_id = $1", payment1.ID)
				require.NoError(t, assertErr)
				assert.Nil(t, transferRequest.CircleTransferID)
				assert.Nil(t, transferRequest.SourceWalletID)

				// Transfer request is updated for the transaction
				assertErr = sqlExecuter.GetContext(ctx, &transferRequest, "SELECT * FROM circle_transfer_requests WHERE payment_id = $1", payment1.ID)
				require.NoError(t, assertErr)
				assert.Equal(t, circleTransferID, *transferRequest.CircleTransferID)
				assert.Equal(t, circleWalletID, *transferRequest.SourceWalletID)
				assert.Equal(t, data.CircleTransferStatusPending, *transferRequest.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbtx, runErr := dbConnectionPool.BeginTxx(ctx, nil)
			require.NoError(t, runErr)

			// Teardown
			defer func() {
				err = dbtx.Rollback()
				require.NoError(t, err)

				_, err = dbConnectionPool.ExecContext(ctx, "DELETE FROM circle_transfer_requests")
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

			if tt.fnSetup != nil {
				tt.fnSetup(t, mCircleService)
			}
			runErr = dispatcher.DispatchPayments(ctx, dbtx, tenantID, tt.paymentsToDispatch)
			if tt.wantErr != nil {
				assert.ErrorContains(t, runErr, tt.wantErr.Error())
			} else {
				assert.NoError(t, runErr)
			}

			if tt.fnAsserts != nil {
				tt.fnAsserts(t, dbtx)
			}
		})
	}
}

func Test_CirclePaymentTransferDispatcher_SupportedPlatform(t *testing.T) {
	dispatcher := CirclePaymentTransferDispatcher{}
	assert.Equal(t, schema.CirclePlatform, dispatcher.SupportedPlatform())
}
