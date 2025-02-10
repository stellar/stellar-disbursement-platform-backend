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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_CirclePaymentTransferDispatcher_DispatchPayments(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	tenantID := "tenant-id"
	tnt := tenant.Tenant{
		ID:      tenantID,
		BaseURL: utils.Ptr("https://example.com"),
	}

	ctx := context.Background()
	ctx = tenant.SaveTenantInContext(ctx, &tnt)
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

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
	err = models.ReceiverWallet.Update(ctx, rwWithoutMemo.ID, data.ReceiverWalletUpdate{
		StellarMemo:     utils.Ptr(""),
		StellarMemoType: utils.Ptr(schema.MemoType("")),
	}, dbConnectionPool)
	require.NoError(t, err)
	paymentWithoutMemo := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwWithoutMemo,
		Disbursement:   disbursementB,
		Asset:          *disbursementB.Asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})

	t.Run("🔴", func(t *testing.T) {
		failureTestCases := []struct {
			name               string
			paymentsToDispatch []*data.Payment
			wantErr            error
			fnSetup            func(*testing.T, *circle.MockService)
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
				paymentsToDispatch: []*data.Payment{paymentWithMemo},
				wantErr:            nil,
				fnSetup: func(t *testing.T, m *circle.MockService) {
					transferRequest, setupErr := models.CircleTransferRequests.Insert(ctx, paymentWithMemo.ID)
					require.NoError(t, setupErr)

					m.On("SendTransfer", ctx, circle.PaymentRequest{
						APIType:                   circle.APITypeTransfers,
						SourceWalletID:            circleWalletID,
						DestinationStellarAddress: paymentWithMemo.ReceiverWallet.StellarAddress,
						DestinationStellarMemo:    paymentWithMemo.ReceiverWallet.StellarMemo,
						Amount:                    paymentWithMemo.Amount,
						StellarAssetCode:          paymentWithMemo.Asset.Code,
						IdempotencyKey:            transferRequest.IdempotencyKey,
					}).
						Return(nil, fmt.Errorf("error posting transfer to Circle")).
						Once()
				},
			},
			{
				name:               "error updating circle transfer request",
				paymentsToDispatch: []*data.Payment{paymentWithMemo},
				wantErr:            fmt.Errorf("updating circle transfer request: transfer cannot be nil"),
				fnSetup: func(t *testing.T, m *circle.MockService) {
					m.On("SendTransfer", ctx, mock.AnythingOfType("circle.PaymentRequest")).
						Return(nil, nil).
						Once()
				},
			},
			{
				name:               "error updating payment status for completed request",
				paymentsToDispatch: []*data.Payment{paymentWithMemo},
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
		}

		for _, tc := range failureTestCases {
			t.Run(tc.name, func(t *testing.T) {
				dbtx, runErr := dbConnectionPool.BeginTxx(ctx, nil)
				require.NoError(t, runErr)
				defer func() {
					err = dbtx.Rollback()
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

				runErr = dispatcher.DispatchPayments(ctx, dbtx, tenantID, tc.paymentsToDispatch)
				if tc.wantErr != nil {
					assert.ErrorContains(t, runErr, tc.wantErr.Error())
				} else {
					assert.NoError(t, runErr)
				}
			})
		}
	})

	t.Run("🟢", func(t *testing.T) {
		successfulTestCases := []struct {
			name                string
			IsTenantMemoEnabled bool
			paymentToDispatch   *data.Payment
			fnAssertMemo        func(t *testing.T, p data.Payment, pReq circle.PaymentRequest)
		}{
			{
				name:                "success posting transfer to Circle Transfers with ReceiverWallet memo",
				IsTenantMemoEnabled: false,
				paymentToDispatch:   paymentWithMemo,
				fnAssertMemo: func(t *testing.T, p data.Payment, pReq circle.PaymentRequest) {
					assert.Equal(t, p.ReceiverWallet.StellarMemo, pReq.DestinationStellarMemo)
				},
			},
			{
				name:                "success posting transfer to Circle Transfers without ReceiverWallet nor Organization memo",
				IsTenantMemoEnabled: false,
				paymentToDispatch:   paymentWithoutMemo,
				fnAssertMemo: func(t *testing.T, p data.Payment, pReq circle.PaymentRequest) {
					assert.Empty(t, pReq.DestinationStellarMemo)
				},
			},
			{
				name:                "success posting transfer to Circle Transfers with Organization memo enabled",
				IsTenantMemoEnabled: true,
				paymentToDispatch:   paymentWithoutMemo,
				fnAssertMemo: func(t *testing.T, p data.Payment, pReq circle.PaymentRequest) {
					assert.Equal(t, GenerateHashFromBaseURL(*tnt.BaseURL), pReq.DestinationStellarMemo)
				},
			},
		}

		for _, tc := range successfulTestCases {
			t.Run(tc.name, func(t *testing.T) {
				err = models.Organizations.Update(ctx, &data.OrganizationUpdate{IsTenantMemoEnabled: utils.Ptr(tc.IsTenantMemoEnabled)})
				require.NoError(t, err)

				dbtx, runErr := dbConnectionPool.BeginTxx(ctx, nil)
				require.NoError(t, runErr)
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

				runErr = dispatcher.DispatchPayments(ctx, dbtx, tenantID, []*data.Payment{p})
				assert.NoError(t, runErr)

				// Payment should be marked as pending
				paymentFromDB, assertErr := models.Payment.Get(ctx, p.ID, dbtx)
				require.NoError(t, assertErr)
				assert.Equal(t, data.PendingPaymentStatus, paymentFromDB.Status)

				// Transfer request is still not updated for the main connection pool
				var transferRequest data.CircleTransferRequest
				assertErr = dbConnectionPool.GetContext(ctx, &transferRequest, "SELECT * FROM circle_transfer_requests WHERE payment_id = $1", p.ID)
				require.NoError(t, assertErr)
				assert.Nil(t, transferRequest.CircleTransferID)
				assert.Nil(t, transferRequest.SourceWalletID)

				// Transfer request is updated for the transaction
				assertErr = dbtx.GetContext(ctx, &transferRequest, "SELECT * FROM circle_transfer_requests WHERE payment_id = $1", p.ID)
				require.NoError(t, assertErr)
				assert.Equal(t, circleTransferID, *transferRequest.CircleTransferID)
				assert.Equal(t, circleWalletID, *transferRequest.SourceWalletID)
				assert.Equal(t, data.CircleTransferStatusPending, *transferRequest.Status)
			})
		}
	})
}

func Test_CirclePaymentTransferDispatcher_SupportedPlatform(t *testing.T) {
	dispatcher := CirclePaymentTransferDispatcher{}
	assert.Equal(t, schema.CirclePlatform, dispatcher.SupportedPlatform())
}
