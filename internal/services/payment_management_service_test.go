package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
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
	ctx := sdpcontext.SetTokenInContext(context.Background(), token)

	service := NewPaymentManagementService(models, models.DBConnectionPool)

	// create fixtures
	wallet := data.CreateDefaultWalletFixture(t, ctx, dbConnectionPool)
	asset := data.GetAssetFixture(t, ctx, dbConnectionPool, data.FixtureAssetUSDC)

	// create disbursements
	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &data.Disbursement{
		Name:   "ready disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
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

	t.Run("payment taken by the scheduler while the cancel waits for the row lock", func(t *testing.T) {
		receiver5 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		rw5 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver5.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
		racedPayment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &data.Payment{
			ReceiverWallet: rw5,
			Disbursement:   startedDisbursement,
			Asset:          *asset,
			Amount:         "500",
			Status:         data.ReadyPaymentStatus,
		})

		// scheduler side: lock the row the way SendBatchPayments does
		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = dbTx.Rollback() }() // releases the lock if an assertion fails before Commit
		batch, err := models.Payment.GetBatchForUpdate(ctx, dbTx, 10)
		require.NoError(t, err)
		var locked *data.Payment
		for _, p := range batch {
			if p.ID == racedPayment.ID {
				locked = p
			}
		}
		require.NotNil(t, locked, "raced payment should be in the locked batch")

		// cancel side: reads READY, then its UPDATE must block on the locked row
		cancelErr := make(chan error, 1)
		go func() { cancelErr <- service.CancelPayment(ctx, racedPayment.ID) }()
		require.Eventually(t, func() bool {
			var waiting int
			qErr := dbConnectionPool.GetContext(ctx, &waiting, `SELECT count(*) FROM pg_stat_activity
				WHERE datname = current_database() AND wait_event_type = 'Lock' AND query ILIKE '%UPDATE payments%'`)
			return qErr == nil && waiting == 1
		}, 5*time.Second, 50*time.Millisecond, "cancel's UPDATE should be waiting on the row lock")

		// scheduler commits PENDING, releasing the lock
		numUpdated, err := models.Payment.UpdateStatuses(ctx, dbTx, []*data.Payment{locked}, data.PendingPaymentStatus)
		require.NoError(t, err)
		require.Equal(t, int64(1), numUpdated)
		require.NoError(t, dbTx.Commit())

		select {
		case err = <-cancelErr:
			require.ErrorIs(t, err, ErrPaymentNotReadyToCancel)
		case <-time.After(5 * time.Second):
			require.FailNow(t, "cancel did not return after the lock was released")
		}

		payment, err := models.Payment.Get(ctx, racedPayment.ID, models.DBConnectionPool)
		require.NoError(t, err)
		assert.Equal(t, data.PendingPaymentStatus, payment.Status)
		for _, entry := range payment.StatusHistory {
			assert.NotEqual(t, data.CanceledPaymentStatus, entry.Status)
		}
	})
}
