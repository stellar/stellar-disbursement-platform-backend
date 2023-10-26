package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewPatchAnchorPlatformTransactionCompletionService(t *testing.T) {
	svc, err := NewPatchAnchorPlatformTransactionCompletionService(nil, nil)
	assert.EqualError(t, err, "anchor platform API service is required")
	assert.Nil(t, svc)

	svc, err = NewPatchAnchorPlatformTransactionCompletionService(&anchorplatform.AnchorPlatformAPIServiceMock{}, nil)
	assert.EqualError(t, err, "SDP models are required")
	assert.Nil(t, svc)

	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	svc, err = NewPatchAnchorPlatformTransactionCompletionService(&anchorplatform.AnchorPlatformAPIServiceMock{}, models)
	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func Test_PatchAnchorPlatformTransactionCompletionService_PatchTransactionsCompletion(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	apAPISvcMock := anchorplatform.AnchorPlatformAPIServiceMock{}
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	svc, err := NewPatchAnchorPlatformTransactionCompletionService(&apAPISvcMock, models)
	require.NoError(t, err)

	getAPTransactionSyncedAt := func(t *testing.T, ctx context.Context, conn db.DBConnectionPool, receiverWalletID string) time.Time {
		const q = "SELECT anchor_platform_transaction_synced_at FROM receiver_wallets WHERE id = $1"
		var syncedAt pq.NullTime
		err := conn.GetContext(ctx, &syncedAt, q, receiverWalletID)
		require.NoError(t, err)
		return syncedAt.Time
	}

	t.Run("doesn't patch transactions when there are no Success or Failed payments", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		err := svc.PatchTransactionsCompletion(ctx)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: got 0 payments to process", entries[0].Message)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for 0 receiver wallet(s)", entries[1].Message)
	})

	t.Run("doesn't mark as synced when fails patching anchor platform transaction when payment is success", func(t *testing.T) {
		data.DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostSuccessCompletion", ctx, anchorplatform.APSep24TransactionPatchPostSuccess{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
				StellarTransactions: []anchorplatform.APStellarTransaction{
					{
						ID:       payment.StellarTransactionID,
						Memo:     receiverWallet.StellarMemo,
						MemoType: receiverWallet.StellarMemoType,
					},
				},
				CompletedAt: &payment.UpdatedAt,
				AmountOut: anchorplatform.APAmount{
					Amount: payment.Amount,
					Asset:  anchorplatform.NewStellarAssetInAIF(payment.Asset.Code, payment.Asset.Issuer),
				},
			}).
			Return(anchorplatform.ErrInvalidToken).
			Once()

		err := svc.PatchTransactionsCompletion(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.True(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 3)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: got 1 payments to process", entries[0].Message)
		assert.Equal(t, fmt.Sprintf(`PatchAnchorPlatformTransactionService: error patching anchor transaction ID %q with status %q: invalid token`, receiverWallet.AnchorPlatformTransactionID, anchorplatform.APTransactionStatusCompleted), entries[1].Message)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for 0 receiver wallet(s)", entries[2].Message)
	})

	t.Run("mark as synced when patch anchor platform transaction successfully and payment is failed", func(t *testing.T) {
		data.DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.PendingPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		errorMsg := "tx_failed op_no_source_account"
		err := models.Payment.Update(ctx, dbConnectionPool, payment, &data.PaymentUpdate{
			Status:               data.FailedPaymentStatus,
			StatusMessage:        errorMsg,
			StellarTransactionID: "stellar-transaction-id",
		})
		require.NoError(t, err)

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostErrorCompletion", ctx, anchorplatform.APSep24TransactionPatchPostError{
				ID:      receiverWallet.AnchorPlatformTransactionID,
				SEP:     "24",
				Message: errorMsg,
				Status:  anchorplatform.APTransactionStatusError,
			}).
			Return(nil).
			Once()

		err = svc.PatchTransactionsCompletion(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.False(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: got 1 payments to process", entries[0].Message)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for 1 receiver wallet(s)", entries[1].Message)
	})

	t.Run("marks as synced when patch anchor platform transaction successfully and payment is success", func(t *testing.T) {
		data.DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostSuccessCompletion", ctx, anchorplatform.APSep24TransactionPatchPostSuccess{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
				StellarTransactions: []anchorplatform.APStellarTransaction{
					{
						ID:       payment.StellarTransactionID,
						Memo:     receiverWallet.StellarMemo,
						MemoType: receiverWallet.StellarMemoType,
					},
				},
				CompletedAt: &payment.UpdatedAt,
				AmountOut: anchorplatform.APAmount{
					Amount: payment.Amount,
					Asset:  anchorplatform.NewStellarAssetInAIF(payment.Asset.Code, payment.Asset.Issuer),
				},
			}).
			Return(nil).
			Once()

		err := svc.PatchTransactionsCompletion(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.False(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: got 1 payments to process", entries[0].Message)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for 1 receiver wallet(s)", entries[1].Message)
	})

	t.Run("doesn't patch the transaction when it's already patch as completed", func(t *testing.T) {
		data.DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement1,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.FailedPaymentStatus,
			Disbursement:         disbursement2,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostSuccessCompletion", ctx, anchorplatform.APSep24TransactionPatchPostSuccess{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
				StellarTransactions: []anchorplatform.APStellarTransaction{
					{
						ID:       payment.StellarTransactionID,
						Memo:     receiverWallet.StellarMemo,
						MemoType: receiverWallet.StellarMemoType,
					},
				},
				CompletedAt: &payment.UpdatedAt,
				AmountOut: anchorplatform.APAmount{
					Amount: payment.Amount,
					Asset:  anchorplatform.NewStellarAssetInAIF(payment.Asset.Code, payment.Asset.Issuer),
				},
			}).
			Return(nil).
			Once()

		err := svc.PatchTransactionsCompletion(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.False(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 3)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: got 2 payments to process", entries[0].Message)
		assert.Equal(t,
			fmt.Sprintf(`PatchAnchorPlatformTransactionService: anchor platform transaction ID %q already patched as completed. No action needed`, receiverWallet.AnchorPlatformTransactionID),
			entries[1].Message)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for 1 receiver wallet(s)", entries[2].Message)
	})

	t.Run("patches the transactions successfully if the other payments were failed", func(t *testing.T) {
		data.DeleteAllFixtures(t, ctx, dbConnectionPool)

		country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
		asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

		disbursement1 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		disbursement2 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		disbursement3 := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
			Country:           country,
			Wallet:            wallet,
			Asset:             asset,
			Status:            data.StartedDisbursementStatus,
			VerificationField: data.VerificationFieldDateOfBirth,
		})

		payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.PendingPaymentStatus,
			Disbursement:         disbursement1,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-2",
			StellarOperationID:   "operation-id-1",
			Status:               data.PendingPaymentStatus,
			Disbursement:         disbursement2,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		payment3 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-3",
			StellarOperationID:   "operation-id-1",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement3,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		errorMsg := "tx_failed op_no_source_account"
		err := models.Payment.Update(ctx, dbConnectionPool, payment1, &data.PaymentUpdate{
			Status:               data.FailedPaymentStatus,
			StatusMessage:        errorMsg,
			StellarTransactionID: "stellar-transaction-id-1",
		})
		require.NoError(t, err)

		bigErrorMsg := strings.Repeat("tx_failed op_no_source_account", 100)
		err = models.Payment.Update(ctx, dbConnectionPool, payment2, &data.PaymentUpdate{
			Status:               data.FailedPaymentStatus,
			StatusMessage:        bigErrorMsg,
			StellarTransactionID: "stellar-transaction-id-2",
		})
		require.NoError(t, err)

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostErrorCompletion", ctx, anchorplatform.APSep24TransactionPatchPostError{
				ID:      receiverWallet.AnchorPlatformTransactionID,
				SEP:     "24",
				Message: errorMsg,
				Status:  anchorplatform.APTransactionStatusError,
			}).
			Return(nil).
			Once().
			On("PatchAnchorTransactionsPostErrorCompletion", ctx, anchorplatform.APSep24TransactionPatchPostError{
				ID:      receiverWallet.AnchorPlatformTransactionID,
				SEP:     "24",
				Message: bigErrorMsg[:MaxErrorMessageLength-1],
				Status:  anchorplatform.APTransactionStatusError,
			}).
			Return(nil).
			Once().
			On("PatchAnchorTransactionsPostSuccessCompletion", ctx, anchorplatform.APSep24TransactionPatchPostSuccess{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
				StellarTransactions: []anchorplatform.APStellarTransaction{
					{
						ID:       payment3.StellarTransactionID,
						Memo:     receiverWallet.StellarMemo,
						MemoType: receiverWallet.StellarMemoType,
					},
				},
				CompletedAt: &payment3.UpdatedAt,
				AmountOut: anchorplatform.APAmount{
					Amount: payment3.Amount,
					Asset:  anchorplatform.NewStellarAssetInAIF(payment3.Asset.Code, payment3.Asset.Issuer),
				},
			}).
			Return(nil).
			Once()

		err = svc.PatchTransactionsCompletion(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.False(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: got 3 payments to process", entries[0].Message)
		assert.Equal(t, "PatchAnchorPlatformTransactionService: updating anchor platform transaction synced at for 3 receiver wallet(s)", entries[1].Message)
	})

	apAPISvcMock.AssertExpectations(t)
}
