package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

func Test_EmbeddedWalletService_CreateWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tssModel := store.NewTransactionModel(dbConnectionPool)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash)

	defaultTenantID := "test-tenant-id"
	defaultPublicKey := "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23"

	t.Run("successfully creates a wallet TSS transaction", func(t *testing.T) {
		defer data.DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", defaultTenantID, "", "", data.PendingWalletStatus)
		walletIDForTest := initialWallet.Token

		err := service.CreateWallet(ctx, defaultTenantID, walletIDForTest, defaultPublicKey)
		require.NoError(t, err)

		updatedWallet, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, walletIDForTest)
		require.NoError(t, err)
		require.NotNil(t, updatedWallet)
		assert.Equal(t, data.ProcessingWalletStatus, updatedWallet.WalletStatus)
		assert.Equal(t, testWasmHash, updatedWallet.WasmHash)

		expectedExternalID := "wallet_" + walletIDForTest
		transactions, err := tssModel.GetAllByPaymentIDs(ctx, []string{expectedExternalID})
		require.NoError(t, err)
		require.Len(t, transactions, 1)

		tssTransaction := transactions[0]
		assert.Equal(t, expectedExternalID, tssTransaction.ExternalID)
		assert.Equal(t, store.TransactionTypeWalletCreation, tssTransaction.TransactionType)
		assert.Equal(t, defaultTenantID, tssTransaction.TenantID)
		assert.Equal(t, defaultPublicKey, tssTransaction.WalletCreation.PublicKey)
		assert.Equal(t, testWasmHash, tssTransaction.WalletCreation.WasmHash)
	})

	t.Run("returns error if token (walletID) is empty", func(t *testing.T) {
		err := service.CreateWallet(ctx, defaultTenantID, "", defaultPublicKey)
		assert.EqualError(t, err, "token is required")
	})

	t.Run("returns error if publicKey is empty", func(t *testing.T) {
		err := service.CreateWallet(ctx, defaultTenantID, "123", "")
		assert.EqualError(t, err, "public key is required")
	})

	t.Run("returns error if GetByToken fails (wallet not found)", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
		nonExistentToken := "non-existent-token"
		err := service.CreateWallet(ctx, defaultTenantID, nonExistentToken, defaultPublicKey)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Contains(t, err.Error(), "transaction execution error: token does not exist")
	})

	t.Run("returns error if wallet status is not pending", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", defaultTenantID, "", "", data.SuccessWalletStatus)
		walletIDForTest := initialWallet.Token

		err := service.CreateWallet(ctx, defaultTenantID, walletIDForTest, defaultPublicKey)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCreateWalletInvalidStatus)
		assert.Contains(t, err.Error(), "transaction execution error: wallet status is not pending for token")
	})

	t.Run("rolls back wallet update if TSS transaction creation fails", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		invalidService := NewEmbeddedWalletService(sdpModels, tssModel, "invalid_hash_not_32_bytes")

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", defaultTenantID, "", "", data.PendingWalletStatus)
		walletIDForTest := initialWallet.Token

		err := invalidService.CreateWallet(ctx, defaultTenantID, walletIDForTest, defaultPublicKey)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating wallet transaction in TSS")

		unchangedWallet, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, walletIDForTest)
		require.NoError(t, err)
		assert.Equal(t, data.PendingWalletStatus, unchangedWallet.WalletStatus)
		assert.Empty(t, unchangedWallet.WasmHash)
	})
}

func Test_EmbeddedWalletService_GetWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tssModel := store.NewTransactionModel(dbConnectionPool)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash)
	defaultTenantID := "test-tenant-id"

	t.Run("successfully gets a wallet", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		expectedWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", defaultTenantID, "somehash", "somecontract", data.SuccessWalletStatus)

		retrievedWallet, err := service.GetWallet(ctx, defaultTenantID, expectedWallet.Token)
		require.NoError(t, err)
		require.NotNil(t, retrievedWallet)

		assert.Equal(t, expectedWallet.Token, retrievedWallet.Token)
		assert.Equal(t, expectedWallet.TenantID, retrievedWallet.TenantID)
		assert.Equal(t, expectedWallet.WasmHash, retrievedWallet.WasmHash)
		assert.Equal(t, expectedWallet.ContractAddress, retrievedWallet.ContractAddress)
		assert.Equal(t, expectedWallet.WalletStatus, retrievedWallet.WalletStatus)
		assert.NotNil(t, retrievedWallet.CreatedAt)
		assert.NotNil(t, retrievedWallet.UpdatedAt)
	})

	t.Run("returns error if token (walletID) is empty", func(t *testing.T) {
		_, err := service.GetWallet(ctx, defaultTenantID, "")
		assert.EqualError(t, err, "token is required")
	})

	t.Run("returns error if GetByToken fails (wallet not found)", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
		nonExistentToken := "non-existent-token"
		_, err := service.GetWallet(ctx, defaultTenantID, nonExistentToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Contains(t, err.Error(), "token does not exist")
	})

	t.Run("returns error if tenant ID does not match the wallet's tenant ID", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		expectedWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "other-tenant-id", "somehash", "somecontract", data.SuccessWalletStatus)

		_, err := service.GetWallet(ctx, defaultTenantID, expectedWallet.Token)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGetWalletMismatchedTenant)
		assert.Contains(t, err.Error(), "tenant ID does not match the wallet's tenant ID")
	})
}
