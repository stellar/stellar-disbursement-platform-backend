package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_Fixtures_CreateTransactionFixture(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	tx := Transaction{
		Payment: Payment{
			AssetCode:   "USDC",
			AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
			Amount:      1,
		},
	}

	t.Run("create transaction with pending status", func(t *testing.T) {
		tx.ExternalID = uuid.NewString()
		createdTx := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
			ExternalID:         tx.ExternalID,
			AssetCode:          tx.AssetCode,
			AssetIssuer:        tx.AssetIssuer,
			DestinationAddress: tx.Destination,
			Status:             TransactionStatusPending,
			Amount:             tx.Amount,
			TenantID:           uuid.NewString(),
		})
		assert.Equal(t, tx.AssetCode, createdTx.AssetCode)
		assert.Equal(t, tx.AssetIssuer, createdTx.AssetIssuer)
		assert.Equal(t, tx.ExternalID, createdTx.ExternalID)
		assert.Equal(t, tx.Amount, createdTx.Amount)
		assert.Empty(t, createdTx.CompletedAt)
	})

	t.Run("create transaction with successful status", func(t *testing.T) {
		tx.ExternalID = uuid.NewString()
		createdTx := CreateTransactionFixtureNew(t, ctx, dbConnectionPool, TransactionFixture{
			ExternalID:         tx.ExternalID,
			AssetCode:          tx.AssetCode,
			AssetIssuer:        tx.AssetIssuer,
			DestinationAddress: tx.Destination,
			Status:             TransactionStatusSuccess,
			Amount:             tx.Amount,
		})
		assert.Equal(t, tx.AssetCode, createdTx.AssetCode)
		assert.Equal(t, tx.AssetIssuer, createdTx.AssetIssuer)
		assert.Equal(t, tx.ExternalID, createdTx.ExternalID)
		assert.Equal(t, tx.Amount, createdTx.Amount)
		assert.False(t, createdTx.CompletedAt.IsZero())
	})
}

func Test_Fixtures_CreateAndDeleteAllTransactionFixtures(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	tx := Transaction{
		ExternalID: "external-id-1",
		Payment: Payment{
			AssetCode:   "USDC",
			AssetIssuer: "GCBIRB7Q5T53H4L6P5QSI3O6LPD5MBWGM5GHE7A5NY4XT5OT4VCOEZFX",
			Amount:      1,
		},
	}

	t.Run("create and delete transactions", func(t *testing.T) {
		txCount := 5
		createdTxs := CreateTransactionFixturesNew(t, ctx, dbConnectionPool, txCount, TransactionFixture{
			AssetCode:          tx.AssetCode,
			AssetIssuer:        tx.AssetIssuer,
			DestinationAddress: tx.Destination,
			Status:             TransactionStatusPending,
			Amount:             tx.Amount,
			TenantID:           uuid.NewString(),
		})

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
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	channelAccountsCount := 5
	channelAccounts := CreateChannelAccountFixtures(t, ctx, dbConnectionPool, channelAccountsCount)
	assert.Len(t, channelAccounts, channelAccountsCount)
}
