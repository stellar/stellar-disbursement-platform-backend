package services

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetPaymentsWithCount(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	service := NewPaymentService(models, dbConnectionPool)

	t.Run("0 payments created", func(t *testing.T) {
		response, err := service.GetPaymentsWithCount(ctx, &data.QueryParams{})
		require.NoError(t, err)

		assert.Equal(t, response.TotalPayments, 0)
		assert.Equal(t, response.Payments, []data.Payment(nil))
	})

	t.Run("error invalid payment status", func(t *testing.T) {
		_, err := service.GetPaymentsWithCount(ctx, &data.QueryParams{
			Filters: map[data.FilterKey]interface{}{
				data.FilterKeyStatus: "INVALID",
			},
		})
		require.EqualError(t, err, `running atomic function in RunInTransactionWithResult: error counting payments: error counting payments: pq: invalid input value for enum payment_status: "INVALID"`)
	})

	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := data.CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.DraftReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "disbursement 1",
		Status:  data.DraftDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		Amount: "50",
		Status: data.DraftPaymentStatus,
		StatusHistory: []data.PaymentStatusHistoryEntry{
			{
				Status:        data.DraftPaymentStatus,
				StatusMessage: "",
				Timestamp:     time.Now(),
			},
		},
		Disbursement:   disbursement,
		Asset:          *asset,
		ReceiverWallet: receiverWallet,
	})

	t.Run("return payment", func(t *testing.T) {
		response, err := service.GetPaymentsWithCount(ctx, &data.QueryParams{})
		require.NoError(t, err)

		assert.Equal(t, response.TotalPayments, 1)
		assert.Equal(t, response.Payments, []data.Payment{*payment})
	})

	t.Run("return multiple payments", func(t *testing.T) {
		payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			Amount: "50",
			Status: data.DraftPaymentStatus,
			StatusHistory: []data.PaymentStatusHistoryEntry{
				{
					Status:        data.DraftPaymentStatus,
					StatusMessage: "",
					Timestamp:     time.Now(),
				},
			},
			Disbursement:   disbursement,
			Asset:          *asset,
			ReceiverWallet: receiverWallet,
		})

		response, err := service.GetPaymentsWithCount(ctx, &data.QueryParams{})
		require.NoError(t, err)

		assert.Equal(t, response.TotalPayments, 2)
		assert.Equal(t, response.Payments, []data.Payment{*payment2, *payment})
	})
}
