package services

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stretchr/testify/require"
)

func Test_PaymentManagementService_CancelPayment(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	models, outerErr := data.NewModels(dbConnectionPool)
	require.NoError(t, outerErr)

	token := "token"
	ctx := context.WithValue(context.Background(), middleware.TokenContextKey, token)

	service := NewPaymentManagementService(models, models.DBConnectionPool)

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)
	country := data.GetCountryFixture(t, ctx, dbConnectionPool, data.FixtureCountryUSA)

	// create disbursements
	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:    "ready disbursement",
		Status:  data.StartedDisbursementStatus,
		Asset:   asset,
		Wallet:  wallet,
		Country: country,
	})

	// create disbursement receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver3 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver4 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	rw1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw3 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rwReady := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver4.ID, wallet.ID, data.ReadyReceiversWalletStatus)

	readyPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw1,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})
	draftPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw2,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.DraftPaymentStatus,
	})
	pausedPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rw3,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "300",
		Status:         data.PausedPaymentStatus,
	})
	pendingPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
		ReceiverWallet: rwReady,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "400",
		Status:         data.PendingPaymentStatus,
	})

	t.Run("payment doesn't exist", func(t *testing.T) {
		id := "5e1f1c7f5b6c9c0001c1b1b1"

		err := service.CancelPayment(ctx, id)
		require.ErrorIs(t, err, ErrPaymentNotFound)
	})

	t.Run("payment not ready to cancel", func(t *testing.T) {
		err := service.CancelPayment(ctx, draftPayment.ID)
		require.ErrorIs(t, err, ErrPaymentNotReadyToCancel)

		err = service.CancelPayment(ctx, pausedPayment.ID)
		require.ErrorIs(t, err, ErrPaymentNotReadyToCancel)

		err = service.CancelPayment(ctx, pendingPayment.ID)
		require.ErrorIs(t, err, ErrPaymentNotReadyToCancel)
	})

	t.Run("payment canceled", func(t *testing.T) {
		err := service.CancelPayment(ctx, readyPayment.ID)
		require.NoError(t, err)

		payment, err := models.Payment.Get(ctx, readyPayment.ID, models.DBConnectionPool)
		require.NoError(t, err)
		require.Equal(t, data.CanceledPaymentStatus, payment.Status)
	})
}
