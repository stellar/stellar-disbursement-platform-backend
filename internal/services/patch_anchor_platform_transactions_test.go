package services

import (
	"context"
	"fmt"
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

func Test_NewPatchAnchorPlatformTransactionService(t *testing.T) {
	svc, err := NewPatchAnchorPlatformTransactionService(nil, nil)
	assert.EqualError(t, err, "anchor platform API service is required")
	assert.Nil(t, svc)

	svc, err = NewPatchAnchorPlatformTransactionService(&anchorplatform.AnchorPlatformAPIServiceMock{}, nil)
	assert.EqualError(t, err, "SDP models are required")
	assert.Nil(t, svc)

	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	svc, err = NewPatchAnchorPlatformTransactionService(&anchorplatform.AnchorPlatformAPIServiceMock{}, models)
	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func Test_PatchAnchorPlatformTransactionService_PatchTransactions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	apAPISvcMock := anchorplatform.AnchorPlatformAPIServiceMock{}
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	svc, err := NewPatchAnchorPlatformTransactionService(&apAPISvcMock, models)
	require.NoError(t, err)

	getAPTransactionSyncedAt := func(t *testing.T, ctx context.Context, conn db.DBConnectionPool, receiverWalletID string) time.Time {
		const q = "SELECT anchor_platform_transaction_synced_at FROM receiver_wallets WHERE id = $1"
		var syncedAt pq.NullTime
		err := conn.GetContext(ctx, &syncedAt, q, receiverWalletID)
		require.NoError(t, err)
		return syncedAt.Time
	}

	t.Run("doesn't patch transactions when there are no Success or Failed payments", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := svc.PatchTransactions(ctx)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "got 0 payments to process", entries[0].Message)
		assert.Equal(t, "updating anchor platform transaction synced at for 0 receiver wallet(s)", entries[1].Message)
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

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostRegistration", ctx, anchorplatform.APSep24TransactionPatchPostRegistration{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
			}).
			Return(anchorplatform.ErrInvalidToken).
			Once()

		err := svc.PatchTransactions(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.True(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 3)
		assert.Equal(t, "got 1 payments to process", entries[0].Message)
		assert.Equal(t, fmt.Sprintf(`error patching anchor transaction ID %q: invalid token`, receiverWallet.AnchorPlatformTransactionID), entries[1].Message)
		assert.Equal(t, "updating anchor platform transaction synced at for 0 receiver wallet(s)", entries[2].Message)
	})

	t.Run("doesn't mark as synced when patch anchor platform transaction successfully but payment is failed", func(t *testing.T) {
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

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.FailedPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostRegistration", ctx, anchorplatform.APSep24TransactionPatchPostRegistration{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusError,
			}).
			Once()

		err := svc.PatchTransactions(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.True(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "got 1 payments to process", entries[0].Message)
		assert.Equal(t, "updating anchor platform transaction synced at for 0 receiver wallet(s)", entries[1].Message)
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

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostRegistration", ctx, anchorplatform.APSep24TransactionPatchPostRegistration{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
			}).
			Return(nil).
			Once()

		err := svc.PatchTransactions(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.False(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "got 1 payments to process", entries[0].Message)
		assert.Equal(t, "updating anchor platform transaction synced at for 1 receiver wallet(s)", entries[1].Message)
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

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
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

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostRegistration", ctx, anchorplatform.APSep24TransactionPatchPostRegistration{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
			}).
			Return(nil).
			Once()

		err := svc.PatchTransactions(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.False(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 3)
		assert.Equal(t, "got 2 payments to process", entries[0].Message)
		assert.Equal(t,
			fmt.Sprintf(`anchor platform transaction ID %q already patched as completed. No action needed`, receiverWallet.AnchorPlatformTransactionID),
			entries[1].Message)
		assert.Equal(t, "updating anchor platform transaction synced at for 1 receiver wallet(s)", entries[2].Message)
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

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.FailedPaymentStatus,
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

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.SuccessPaymentStatus,
			Disbursement:         disbursement3,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
		})

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		apAPISvcMock.
			On("PatchAnchorTransactionsPostRegistration", ctx, anchorplatform.APSep24TransactionPatchPostRegistration{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusError,
			}).
			Return(nil).
			Twice().
			On("PatchAnchorTransactionsPostRegistration", ctx, anchorplatform.APSep24TransactionPatchPostRegistration{
				ID:     receiverWallet.AnchorPlatformTransactionID,
				SEP:    "24",
				Status: anchorplatform.APTransactionStatusCompleted,
			}).
			Return(nil).
			Once()

		err := svc.PatchTransactions(ctx)
		require.NoError(t, err)

		syncedAt := getAPTransactionSyncedAt(t, ctx, dbConnectionPool, receiverWallet.ID)
		assert.False(t, syncedAt.IsZero())

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "got 3 payments to process", entries[0].Message)
		assert.Equal(t, "updating anchor platform transaction synced at for 1 receiver wallet(s)", entries[1].Message)
	})

	apAPISvcMock.AssertExpectations(t)
}
