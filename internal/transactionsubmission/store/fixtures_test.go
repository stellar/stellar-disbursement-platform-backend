package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Fixtures_CreateTransactionFixture(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	tx := Transaction{
		AssetCode:   "USDC",
		AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		Amount:      1,
	}

	t.Run("create transaction with pending status", func(t *testing.T) {
		tx.ExternalID = uuid.NewString()
		createdTx := CreateTransactionFixture(
			t,
			ctx,
			dbConnectionPool,
			tx.ExternalID, tx.AssetCode,
			tx.AssetIssuer, tx.Destination,
			TransactionStatusPending, tx.Amount,
		)
		assert.Equal(t, tx.AssetCode, createdTx.AssetCode)
		assert.Equal(t, tx.AssetIssuer, createdTx.AssetIssuer)
		assert.Equal(t, tx.ExternalID, createdTx.ExternalID)
		assert.Equal(t, tx.Amount, createdTx.Amount)
		assert.Empty(t, createdTx.CompletedAt)
	})

	t.Run("create transaction with successful status", func(t *testing.T) {
		tx.ExternalID = uuid.NewString()
		createdTx := CreateTransactionFixture(
			t,
			ctx,
			dbConnectionPool,
			tx.ExternalID, tx.AssetCode,
			tx.AssetIssuer, tx.Destination,
			TransactionStatusSuccess, tx.Amount,
		)
		assert.Equal(t, tx.AssetCode, createdTx.AssetCode)
		assert.Equal(t, tx.AssetIssuer, createdTx.AssetIssuer)
		assert.Equal(t, tx.ExternalID, createdTx.ExternalID)
		assert.Equal(t, tx.Amount, createdTx.Amount)
		assert.False(t, createdTx.CompletedAt.IsZero())
	})
}

func Test_Fixtures_CreateAndDeleteAllTransactionFixtures(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	tx := Transaction{
		ExternalID:  "external-id-1",
		AssetCode:   "USDC",
		AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
		Amount:      1,
	}

	t.Run("create and delete transactions", func(t *testing.T) {
		txCount := 5
		createdTxs := CreateTransactionFixtures(
			t,
			ctx,
			dbConnectionPool,
			txCount, tx.AssetCode,
			tx.AssetIssuer, tx.Destination,
			TransactionStatusPending, tx.Amount,
		)

		assert.Len(t, createdTxs, txCount)
		var createdTxIDs []string
		for _, createdTx := range createdTxs {
			createdTxIDs = append(createdTxIDs, createdTx.ID)
		}

		DeleteAllTransactionFixtures(t, ctx, dbConnectionPool)
		txModel := TransactionModel{DBConnectionPool: dbConnectionPool}

		for _, id := range createdTxIDs {
			tx, err := txModel.Get(ctx, id)
			require.EqualError(t, err, ErrRecordNotFound.Error())
			assert.Nil(t, tx)
		}
	})
}

func Test_Fixtures_CreateChannelAccountsOnChainFixtures(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	channelAccountsCount := 5
	channelAccounts := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, channelAccountsCount)
	assert.Len(t, channelAccounts, channelAccountsCount)
}
