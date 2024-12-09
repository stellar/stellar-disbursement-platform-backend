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

func Test_CircleRecipientModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})

	m := CircleRecipientModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error if receiverWalletID is empty", func(t *testing.T) {
		circleRecipient, err := m.Insert(ctx, "")
		require.EqualError(t, err, "receiverWalletID is required")
		require.Nil(t, circleRecipient)
	})

	t.Run("🎉 successfully inserts a circle recipient", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		circleRecipient, err := m.Insert(ctx, receiverWallet.ID)
		require.NoError(t, err)
		require.NotNil(t, circleRecipient)

		assert.Equal(t, receiverWallet.ID, circleRecipient.ReceiverWalletID)
		assert.NotEmpty(t, circleRecipient.UpdatedAt)
		assert.NotEmpty(t, circleRecipient.CreatedAt)
		assert.Empty(t, circleRecipient.SyncAttempts)
		assert.Nil(t, circleRecipient.LastSyncAttemptAt)
		assert.NoError(t, uuid.Validate(circleRecipient.IdempotencyKey), "idempotency key should be a valid UUID")
	})

	t.Run("database constraint that prevents repeated rows with the same receiverWalletID", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		_, err := m.Insert(ctx, receiverWallet.ID)
		require.NoError(t, err)

		_, err = m.Insert(ctx, receiverWallet.ID)
		require.Error(t, err)
		require.ErrorContains(t, err, "duplicate key value violates unique constraint")
	})
}

func Test_CircleRecipientModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	m := CircleRecipientModel{dbConnectionPool: dbConnectionPool}

	synchedAt := time.Now()
	updateRequest := CircleRecipientUpdate{
		IdempotencyKey:    "new-idempotency-key",
		CircleRecipientID: utils.StringPtr("circle-recipient-id"),
		Status:            utils.Ptr(CircleRecipientStatusActive),
		ResponseBody:      []byte(`{"foo":"bar"}`),
		SyncAttempts:      1,
		LastSyncAttemptAt: &synchedAt,
	}

	t.Run("return an error if the receiverWalletID is empty", func(t *testing.T) {
		circleRecipient, err := m.Update(ctx, "", CircleRecipientUpdate{})
		require.Error(t, err)
		require.ErrorContains(t, err, "receiverWalletID is required")
		require.Nil(t, circleRecipient)
	})

	t.Run("return an error if the circle transfer request does not exist", func(t *testing.T) {
		circleRecipient, err := m.Update(ctx, "test-key", updateRequest)
		require.Error(t, err)
		require.ErrorContains(t, err, "circle recipient with receiver_wallet_id test-key not found")
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Nil(t, circleRecipient)
	})

	t.Run("🎉 successfully updates a circle recipient", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		_, err := m.Insert(ctx, receiverWallet.ID)
		require.NoError(t, err)

		updatedCircleRecipient, err := m.Update(ctx, receiverWallet.ID, updateRequest)
		require.NoError(t, err)
		require.NotNil(t, updatedCircleRecipient)

		assert.Equal(t, updateRequest.IdempotencyKey, updatedCircleRecipient.IdempotencyKey)
		assert.Equal(t, updateRequest.CircleRecipientID, updatedCircleRecipient.CircleRecipientID)
		assert.Equal(t, updateRequest.Status, updatedCircleRecipient.Status)
		assert.JSONEq(t, string(updateRequest.ResponseBody), string(updatedCircleRecipient.ResponseBody))
		assert.Equal(t, updateRequest.SyncAttempts, updatedCircleRecipient.SyncAttempts)
		assert.Truef(t, updateRequest.LastSyncAttemptAt.Equal(*updatedCircleRecipient.LastSyncAttemptAt), "LastSyncAttemptAt doesn't match: %v != %v", updateRequest.LastSyncAttemptAt, updatedCircleRecipient.LastSyncAttemptAt)
	})
}

func Test_CircleRecipientModel_GetByReceiverWalletID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "wallet1", "https://www.wallet.com", "www.wallet.com", "wallet1://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	m := CircleRecipientModel{dbConnectionPool: dbConnectionPool}

	t.Run("return an error if the receiverWalletID is empty", func(t *testing.T) {
		circleRecipient, err := m.GetByReceiverWalletID(ctx, "")
		require.Error(t, err)
		require.ErrorContains(t, err, "receiverWalletID is required")
		require.Nil(t, circleRecipient)
	})

	t.Run("return an error if the circle transfer request does not exist", func(t *testing.T) {
		circleRecipient, err := m.GetByReceiverWalletID(ctx, "test-key")
		require.Error(t, err)
		require.ErrorContains(t, err, "record not found")
		require.ErrorIs(t, err, ErrRecordNotFound)
		require.Nil(t, circleRecipient)
	})

	t.Run("🎉 successfully retrieves a circle recipient", func(t *testing.T) {
		receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)
		defer DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)

		insertedCircleRecipient, err := m.Insert(ctx, receiverWallet.ID)
		require.NoError(t, err)

		fetchedCircleRecipient, err := m.GetByReceiverWalletID(ctx, receiverWallet.ID)
		require.NoError(t, err)
		require.NotNil(t, fetchedCircleRecipient)

		assert.Equal(t, insertedCircleRecipient, fetchedCircleRecipient)
	})
}
