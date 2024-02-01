package data

import (
	"context"
	"strings"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/require"
)

func Test_CreateReceiverFixture(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Create a random receiver
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	require.Len(t, receiver.ID, 36)
	require.NotEmpty(t, receiver.Email)
	require.NotEmpty(t, receiver.PhoneNumber)
	require.NotEmpty(t, receiver.ExternalID)
	require.NotEmpty(t, receiver.CreatedAt)
	require.NotEmpty(t, receiver.UpdatedAt)
}

func Test_CreateReceiverWalletFixture(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Create a random receiver wallet
	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "My Wallet", "https://mywallet.test.com/", "mywallet.test.com", "mtwallet://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	rw := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

	// Check receiver wallet
	require.Len(t, rw.ID, 36)
	require.NotEmpty(t, rw.StellarAddress)
	require.NotEmpty(t, rw.StellarMemo)
	require.NotEmpty(t, rw.StellarMemoType)
	require.Equal(t, DraftReceiversWalletStatus, rw.Status)
	require.Len(t, rw.StatusHistory, 1)
	require.NotEmpty(t, rw.StatusHistory[0].Timestamp)
	require.Equal(t, DraftReceiversWalletStatus, rw.StatusHistory[0].Status)
	require.NotEmpty(t, rw.CreatedAt)
	require.NotEmpty(t, rw.UpdatedAt)

	// Check receiver
	require.Len(t, rw.Receiver.ID, 36)
	require.Equal(t, receiver.ID, rw.Receiver.ID)
	require.NotEmpty(t, rw.Receiver.Email)
	require.NotEmpty(t, rw.Receiver.PhoneNumber)
	require.NotEmpty(t, rw.Receiver.ExternalID)
	require.NotEmpty(t, rw.Receiver.CreatedAt)
	require.NotEmpty(t, rw.Receiver.UpdatedAt)

	// Check wallet
	require.Len(t, rw.Wallet.ID, 36)
	require.Equal(t, wallet.ID, rw.Wallet.ID)
	require.NotEmpty(t, rw.Wallet.Name)
	require.NotEmpty(t, rw.Wallet.Homepage)
	require.NotEmpty(t, rw.Wallet.DeepLinkSchema)
	require.NotEmpty(t, rw.Wallet.CreatedAt)
	require.NotEmpty(t, rw.Wallet.UpdatedAt)
}

func Test_Fixtures_CreateInstructionsFixture(t *testing.T) {
	t.Run("header only for nil instructions", func(t *testing.T) {
		fileContent := CreateInstructionsFixture(t, nil)
		lines := strings.Split(string(fileContent), "\n")
		require.Equal(t, "phone,id,amount,verification_value", lines[0])
	})

	t.Run("header only for empty instructions", func(t *testing.T) {
		buf := CreateInstructionsFixture(t, []*DisbursementInstruction{})
		require.Equal(t, "phone,id,amount,verification_value\n", string(buf))
	})

	t.Run("writes records correctly", func(t *testing.T) {
		instructions := []*DisbursementInstruction{
			{"1234567890", "1", "123.12", "1995-02-20", nil},
			{"0987654321", "2", "321", "1974-07-19", nil},
		}
		buf := CreateInstructionsFixture(t, instructions)
		lines := strings.Split(string(buf), "\n")
		require.Equal(t, "1234567890,1,123.12,1995-02-20", lines[1])
		require.Equal(t, "0987654321,2,321,1974-07-19", lines[2])
	})
}

func Test_Fixtures_UpdateDisbursementInstructionsFixture(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	disbursementModel := &DisbursementModel{dbConnectionPool: dbConnectionPool}

	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, &DisbursementModel{dbConnectionPool: dbConnectionPool}, &Disbursement{
		Name: "disbursement1",
	})

	instructions := []*DisbursementInstruction{
		{"1234567890", "1", "123.12", "1995-02-20", nil},
		{"0987654321", "2", "321", "1974-07-19", nil},
		{"0987654321", "3", "321", "1974-07-19", nil},
	}

	t.Run("update instructions", func(t *testing.T) {
		UpdateDisbursementInstructionsFixture(t, ctx, dbConnectionPool, disbursement.ID, "test.csv", instructions)
		actual, err := disbursementModel.Get(ctx, dbConnectionPool, disbursement.ID)
		require.NoError(t, err)
		require.Equal(t, "test.csv", actual.FileName)
		require.NotEmpty(t, actual.FileContent)
		expected := CreateInstructionsFixture(t, instructions)
		require.Equal(t, expected, actual.FileContent)
	})
}
