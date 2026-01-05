package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_SEP24TransactionModel_Insert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := &SEP24TransactionModel{dbConnectionPool: dbConnectionPool}

	t.Run("🎉 successfully inserts a new SEP24 transaction", func(t *testing.T) {
		transactionID := uuid.New().String()

		transaction, err := model.Insert(ctx, transactionID)
		require.NoError(t, err)
		require.NotNil(t, transaction)

		assert.Equal(t, transactionID, transaction.ID)
		assert.NotZero(t, transaction.CreatedAt)
		assert.WithinDuration(t, time.Now(), transaction.CreatedAt, 5*time.Second)
	})

	t.Run("returns error when transaction ID is empty", func(t *testing.T) {
		transaction, err := model.Insert(ctx, "")
		require.Error(t, err)
		assert.ErrorContains(t, err, "transaction ID is required")
		assert.Nil(t, transaction)
	})

	t.Run("returns ErrRecordAlreadyExists when inserting duplicate transaction ID", func(t *testing.T) {
		transactionID := uuid.New().String()

		// First insert should succeed
		transaction, err := model.Insert(ctx, transactionID)
		require.NoError(t, err)
		require.NotNil(t, transaction)

		// Second insert with same ID should fail with unique violation
		duplicateTransaction, err := model.Insert(ctx, transactionID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRecordAlreadyExists)
		assert.Nil(t, duplicateTransaction)
	})
}

func Test_SEP24TransactionModel_GetByID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := &SEP24TransactionModel{dbConnectionPool: dbConnectionPool}

	t.Run("🎉 successfully retrieves existing SEP24 transaction", func(t *testing.T) {
		transactionID := uuid.New().String()

		// Insert a transaction first
		insertedTransaction, err := model.Insert(ctx, transactionID)
		require.NoError(t, err)
		require.NotNil(t, insertedTransaction)

		// Retrieve it
		retrievedTransaction, err := model.GetByID(ctx, transactionID)
		require.NoError(t, err)
		require.NotNil(t, retrievedTransaction)

		assert.Equal(t, insertedTransaction.ID, retrievedTransaction.ID)
		assert.Equal(t, insertedTransaction.CreatedAt.Unix(), retrievedTransaction.CreatedAt.Unix())
	})

	t.Run("returns ErrRecordNotFound when transaction does not exist", func(t *testing.T) {
		nonExistentID := uuid.New().String()

		transaction, err := model.GetByID(ctx, nonExistentID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, transaction)
	})
}

func Test_SEP24TransactionModel_Insert_UniqueViolationErrorCode(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := &SEP24TransactionModel{dbConnectionPool: dbConnectionPool}

	t.Run("verifies that unique violation error code 23505 is handled correctly", func(t *testing.T) {
		transactionID := uuid.New().String()

		// Insert first transaction
		_, err := model.Insert(ctx, transactionID)
		require.NoError(t, err)

		// Try to insert duplicate - this should trigger PostgreSQL error code 23505
		_, err = model.Insert(ctx, transactionID)
		require.Error(t, err)

		// Verify it's a PostgreSQL unique violation error
		var pqError *pq.Error
		if errors.As(err, &pqError) {
			// This shouldn't happen since we convert it to ErrRecordAlreadyExists
			// But if it does, verify the code
			assert.Equal(t, "23505", pqError.Code)
		}

		// The error should be wrapped as ErrRecordAlreadyExists
		assert.ErrorIs(t, err, ErrRecordAlreadyExists)
	})
}
