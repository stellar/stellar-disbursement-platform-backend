package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
)

func Test_ReadyPaymentsCancellationService_CancelReadyPaymentsService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	service := NewReadyPaymentsCancellationService(models)
	ctx := context.Background()

	data.DeleteAllPaymentsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllDisbursementFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllCountryFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "BRA", "Brazil")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Country:           country,
		Wallet:            wallet,
		Asset:             asset,
		Status:            data.ReadyDisbursementStatus,
		VerificationField: data.VerificationFieldDateOfBirth,
	})

	t.Run("automatic payment cancellation is deactivated", func(t *testing.T) {
		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []data.PaymentStatusHistoryEntry{
				{
					Status:        data.ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
			},
		})

		cancelErr := service.CancelReadyPayments(ctx)
		require.NoError(t, cancelErr)

		entries := getEntries()
		require.Len(t, entries, 1)
		assert.Equal(
			t,
			"automatic ready payment cancellation is deactivated. Set a valid value to the organization's payment_cancellation_period_days to activate it.",
			entries[0].Message,
		)
	})

	// Set the Payment Cancellation Period
	var paymentCancellationPeriod int64 = 5
	err = models.Organizations.Update(ctx, &data.OrganizationUpdate{PaymentCancellationPeriodDays: &paymentCancellationPeriod})
	require.NoError(t, err)

	t.Run("no ready payment for more than 5 days won't cancel any", func(t *testing.T) {
		payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.DraftPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []data.PaymentStatusHistoryEntry{
				{
					Status:        data.DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -6),
				},
			},
		})

		payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []data.PaymentStatusHistoryEntry{
				{
					Status:        data.ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now(),
				},
			},
		})

		cancelErr := service.CancelReadyPayments(ctx)
		require.NoError(t, cancelErr)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		assert.Equal(t, data.DraftPaymentStatus, payment1DB.Status)
		assert.Equal(t, data.ReadyPaymentStatus, payment2DB.Status)
	})

	t.Run("cancels ready payments for more than 5 days", func(t *testing.T) {
		payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []data.PaymentStatusHistoryEntry{
				{
					Status:        data.ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -5),
				},
			},
		})

		payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount:               "1",
			StellarTransactionID: "stellar-transaction-id-1",
			StellarOperationID:   "operation-id-1",
			Status:               data.ReadyPaymentStatus,
			Disbursement:         disbursement,
			ReceiverWallet:       receiverWallet,
			Asset:                *asset,
			StatusHistory: []data.PaymentStatusHistoryEntry{
				{
					Status:        data.ReadyPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now().AddDate(0, 0, -7),
				},
			},
		})

		err := service.CancelReadyPayments(ctx)
		require.NoError(t, err)

		payment1DB, err := models.Payment.Get(ctx, payment1.ID, dbConnectionPool)
		require.NoError(t, err)

		payment2DB, err := models.Payment.Get(ctx, payment2.ID, dbConnectionPool)
		require.NoError(t, err)

		assert.Equal(t, data.CanceledPaymentStatus, payment1DB.Status)
		assert.Equal(t, data.CanceledPaymentStatus, payment2DB.Status)
	})
}
