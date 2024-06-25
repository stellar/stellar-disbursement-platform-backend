package data

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_CircleTransferRequestModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error if paymentID is empty", func(t *testing.T) {
		circleEntry, err := m.Insert(ctx, "")
		require.EqualError(t, err, "paymentID is required")
		require.Nil(t, circleEntry)
	})

	t.Run("🎉 successfully inserts a circle transfer request", func(t *testing.T) {
		paymentID := "payment-id"
		circleEntry, err := m.Insert(ctx, paymentID)
		require.NoError(t, err)
		require.NotNil(t, circleEntry)

		assert.Equal(t, paymentID, circleEntry.PaymentID)
		assert.NotEmpty(t, circleEntry.UpdatedAt)
		assert.NotEmpty(t, circleEntry.CreatedAt)
		assert.Nil(t, circleEntry.CompletedAt)
		assert.NoError(t, uuid.Validate(circleEntry.IdempotencyKey), "idempotency key should be a valid UUID")
	})

	t.Run("database constraint that prevents repeated rows with the same paymentID and status!=failed", func(t *testing.T) {
		paymentID := "payment-id-2"
		circleEntry, err := m.Insert(ctx, paymentID)
		require.NoError(t, err)

		_, err = m.Insert(ctx, paymentID)
		require.Error(t, err)
		require.ErrorContains(t, err, "duplicate key value violates unique constraint")

		// it works again when we update the status of the existing entry to failed
		_, err = m.Update(ctx, dbConnectionPool, circleEntry.IdempotencyKey, CircleTransferRequestUpdate{
			Status:           CircleTransferStatusFailed,
			CircleTransferID: "circle-transfer-id",
			ResponseBody:     []byte(`{"foo":"bar"}`),
			SourceWalletID:   "source-wallet-id",
		})
		require.NoError(t, err)

		_, err = m.Insert(ctx, paymentID)
		require.NoError(t, err)
	})
}

func Test_CircleTransferRequestModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	// idempotencyKey := uuid.NewString()
	updateRequest := CircleTransferRequestUpdate{
		CircleTransferID: "circle-transfer-id",
		Status:           CircleTransferStatusPending,
		ResponseBody:     []byte(`{"foo":"bar"}`),
		SourceWalletID:   "source-wallet-id",
	}

	t.Run("return an error if the idempotencyKey is empty", func(t *testing.T) {
		circleEntry, err := m.Update(ctx, dbConnectionPool, "", CircleTransferRequestUpdate{})
		require.Error(t, err)
		require.ErrorContains(t, err, "idempotencyKey is required")
		require.Nil(t, circleEntry)
	})

	t.Run("return an error if the circle transfer request does not exist", func(t *testing.T) {
		circleEntry, err := m.Update(ctx, dbConnectionPool, "test-key", updateRequest)
		require.Error(t, err)
		require.ErrorContains(t, err, "circle transfer request with idempotency key test-key not found")
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Nil(t, circleEntry)
	})

	t.Run("🎉 successfully updates a circle transfer request (completedAt==nil)", func(t *testing.T) {
		paymentID := "payment-id"
		circleEntry, err := m.Insert(ctx, paymentID)
		require.NoError(t, err)

		updatedCircleEntry, err := m.Update(ctx, dbConnectionPool, circleEntry.IdempotencyKey, updateRequest)
		require.NoError(t, err)
		require.NotNil(t, updatedCircleEntry)

		assert.Equal(t, circleEntry.IdempotencyKey, updatedCircleEntry.IdempotencyKey)
		assert.Equal(t, updateRequest.CircleTransferID, *updatedCircleEntry.CircleTransferID)
		assert.Equal(t, updateRequest.Status, *updatedCircleEntry.Status)
		assert.JSONEq(t, string(updateRequest.ResponseBody), string(updatedCircleEntry.ResponseBody))
		assert.Equal(t, updateRequest.SourceWalletID, *updatedCircleEntry.SourceWalletID)
		assert.NotEmpty(t, updatedCircleEntry.UpdatedAt)
		assert.Nil(t, updatedCircleEntry.CompletedAt)
	})

	t.Run("🎉 successfully updates a circle transfer request(completedAt!=nil)", func(t *testing.T) {
		paymentID := "payment-id2"
		circleEntry, err := m.Insert(ctx, paymentID)
		require.NoError(t, err)

		updateRequest2 := updateRequest
		updateRequest2.CompletedAt = utils.TimePtr(time.Now())
		updatedCircleEntry, err := m.Update(ctx, dbConnectionPool, circleEntry.IdempotencyKey, updateRequest2)
		require.NoError(t, err)
		require.NotNil(t, updatedCircleEntry)

		assert.Equal(t, circleEntry.IdempotencyKey, updatedCircleEntry.IdempotencyKey)
		assert.Equal(t, updateRequest2.CircleTransferID, *updatedCircleEntry.CircleTransferID)
		assert.Equal(t, updateRequest2.Status, *updatedCircleEntry.Status)
		assert.JSONEq(t, string(updateRequest2.ResponseBody), string(updatedCircleEntry.ResponseBody))
		assert.Equal(t, updateRequest2.SourceWalletID, *updatedCircleEntry.SourceWalletID)
		assert.NotEmpty(t, updatedCircleEntry.UpdatedAt)
		assert.NotNil(t, updatedCircleEntry.CompletedAt)
	})
}

func Test_CircleTransferRequestModel_FindIncompleteByPaymentID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	t.Run("return an error if paymentID is empty", func(t *testing.T) {
		circleEntry, err := m.FindIncompleteByPaymentID(ctx, dbConnectionPool, "")
		require.Error(t, err)
		require.ErrorContains(t, err, "paymentID is required")
		require.Nil(t, circleEntry)
	})

	t.Run("return nil if no circle transfer request is found", func(t *testing.T) {
		circleEntry, err := m.FindIncompleteByPaymentID(ctx, dbConnectionPool, "payment-id")
		require.NoError(t, err)
		require.Nil(t, circleEntry)
	})

	t.Run("return nil if the existing circle transfer is in completed_at state", func(t *testing.T) {
		paymentID := "payment-id"
		circleEntry, err := m.Insert(ctx, paymentID)
		require.NoError(t, err)

		_, err = m.Update(ctx, dbConnectionPool, circleEntry.IdempotencyKey, CircleTransferRequestUpdate{
			CircleTransferID: "circle-transfer-id",
			Status:           CircleTransferStatusFailed,
			ResponseBody:     []byte(`{"foo":"bar"}`),
			SourceWalletID:   "source-wallet-id",
			CompletedAt:      utils.TimePtr(time.Now()),
		})
		require.NoError(t, err)

		circleEntry, err = m.FindIncompleteByPaymentID(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)
		require.Nil(t, circleEntry)
	})

	t.Run("🎉 successfully finds an incomplete circle transfer request", func(t *testing.T) {
		paymentID := "payment-id"
		_, err := m.Insert(ctx, paymentID)
		require.NoError(t, err)

		circleEntry, err := m.FindIncompleteByPaymentID(ctx, dbConnectionPool, paymentID)
		require.NoError(t, err)
		require.NotNil(t, circleEntry)
		assert.Equal(t, paymentID, circleEntry.PaymentID)
	})
}

func Test_CircleTransferRequestModel_GetOrInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	m := CircleTransferRequestModel{dbConnectionPool: dbConnectionPool}

	// Create fixtures
	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	country := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, models.Disbursements, &Disbursement{
		Country: country,
		Wallet:  wallet,
		Status:  ReadyDisbursementStatus,
		Asset:   asset,
	})
	receiverReady := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rwReady := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiverReady.ID, wallet.ID, ReadyReceiversWalletStatus)
	payment1 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         DraftPaymentStatus,
	})
	payment2 := CreatePaymentFixture(t, ctx, dbConnectionPool, models.Payment, &Payment{
		ReceiverWallet: rwReady,
		Disbursement:   disbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         DraftPaymentStatus,
	})

	t.Run("return an error if paymentID is empty", func(t *testing.T) {
		circleEntry, err := m.GetOrInsert(ctx, "")
		require.Error(t, err)
		require.ErrorContains(t, err, "paymentID is required")
		require.Nil(t, circleEntry)
	})

	t.Run("🎉 successfully finds an existing circle transfer request", func(t *testing.T) {
		insertedEntry, err := m.Insert(ctx, payment1.ID)
		require.NoError(t, err)

		gotEntry, err := m.GetOrInsert(ctx, payment1.ID)
		require.NoError(t, err)
		require.NotNil(t, gotEntry)
		assert.Equal(t, insertedEntry, gotEntry)
	})

	t.Run("🎉 successfully insert a new circle transfer request", func(t *testing.T) {
		query := "SELECT COUNT(*) FROM circle_transfer_requests"
		var count int
		err := dbConnectionPool.GetContext(ctx, &count, query)
		require.NoError(t, err)
		require.Equal(t, 1, count)

		gotEntry, err := m.GetOrInsert(ctx, payment2.ID)
		require.NoError(t, err)
		require.NotNil(t, gotEntry)
		assert.Equal(t, payment2.ID, gotEntry.PaymentID)

		err = dbConnectionPool.GetContext(ctx, &count, query)
		require.NoError(t, err)
		require.Equal(t, 2, count) // <- new row inserted
	})
}
