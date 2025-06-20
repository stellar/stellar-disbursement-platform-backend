package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_NewEmbeddedWalletService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	wasmHash := "e5da3b"

	t.Run("return an error if sdpModels is nil", func(t *testing.T) {
		tssModel := store.NewTransactionModel(dbConnectionPool)
		service, err := NewEmbeddedWalletService(nil, tssModel, wasmHash)
		assert.Nil(t, service)
		assert.EqualError(t, err, "sdpModels cannot be nil")
	})

	t.Run("return an error if tssModel is nil", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		service, err := NewEmbeddedWalletService(sdpModels, nil, wasmHash)
		assert.Nil(t, service)
		assert.EqualError(t, err, "tssModel cannot be nil")
	})

	t.Run("return an error if wasmHash is empty", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)
		tssModel := store.NewTransactionModel(dbConnectionPool)

		service, err := NewEmbeddedWalletService(sdpModels, tssModel, "")
		assert.Nil(t, service)
		assert.EqualError(t, err, "wasmHash cannot be empty")
	})

	t.Run("🎉 successfully creates a new EmbeddedWalletService instance", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)
		tssModel := store.NewTransactionModel(dbConnectionPool)

		service, err := NewEmbeddedWalletService(sdpModels, tssModel, wasmHash)
		require.NoError(t, err)
		require.NotNil(t, service)

		assert.Equal(t, sdpModels, service.sdpModels)
		assert.Equal(t, tssModel, service.tssModel)
		assert.Equal(t, wasmHash, service.wasmHash)
	})
}

func Test_EmbeddedWalletService_CreateInvitationToken(t *testing.T) {
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

	service, err := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash)
	require.NoError(t, err)

	t.Run("successfully creates unique tokens", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		token1, err := service.CreateInvitationToken(ctx)
		require.NoError(t, err)
		require.NotNil(t, token1)

		assert.NotEmpty(t, token1)

		wallet, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, token1)
		require.NoError(t, err)
		assert.Equal(t, token1, wallet.Token)

		token2, err := service.CreateInvitationToken(ctx)
		require.NoError(t, err)
		require.NotNil(t, token2)

		assert.NotEmpty(t, token2)

		wallet2, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, token2)
		require.NoError(t, err)
		assert.Equal(t, token2, wallet2.Token)

		assert.NotEqual(t, token1, token2, "should generate unique tokens")
	})
}

func Test_EmbeddedWalletService_CreateWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	defaultTenantID := "test-tenant-id"
	ctx := tenant.SaveTenantInContext(context.Background(), &tenant.Tenant{ID: defaultTenantID})

	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tssModel := store.NewTransactionModel(dbConnectionPool)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service, err := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash)
	require.NoError(t, err)

	defaultPublicKey := "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23"
	defaultCredentialID := "test-credential-id"

	t.Run("successfully creates a wallet TSS transaction", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", data.PendingWalletStatus)
		walletIDForTest := initialWallet.Token

		err := service.CreateWallet(ctx, walletIDForTest, defaultPublicKey, defaultCredentialID)
		require.NoError(t, err)

		updatedWallet, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, walletIDForTest)
		require.NoError(t, err)
		require.NotNil(t, updatedWallet)
		assert.Equal(t, data.ProcessingWalletStatus, updatedWallet.WalletStatus)
		assert.Equal(t, testWasmHash, updatedWallet.WasmHash)
		assert.Equal(t, defaultCredentialID, updatedWallet.CredentialID)

		expectedExternalID := walletIDForTest
		transactions, err := tssModel.GetAllByExternalIDs(ctx, []string{expectedExternalID})
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
		err := service.CreateWallet(ctx, "", defaultPublicKey, defaultCredentialID)
		assert.EqualError(t, err, "token is required")
	})

	t.Run("returns error if publicKey is empty", func(t *testing.T) {
		err := service.CreateWallet(ctx, "123", "", defaultCredentialID)
		assert.EqualError(t, err, "public key is required")
	})

	t.Run("returns error if credentialID is empty", func(t *testing.T) {
		err := service.CreateWallet(ctx, "123", defaultPublicKey, "")
		assert.EqualError(t, err, "credential ID is required")
	})

	t.Run("returns error if GetByToken fails (wallet not found)", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
		nonExistentToken := "non-existent-token"
		err := service.CreateWallet(ctx, nonExistentToken, defaultPublicKey, defaultCredentialID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidToken)
		assert.Contains(t, err.Error(), "transaction execution error: token does not exist")
	})

	t.Run("returns error if wallet status is not pending", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", data.SuccessWalletStatus)
		walletIDForTest := initialWallet.Token

		err := service.CreateWallet(ctx, walletIDForTest, defaultPublicKey, defaultCredentialID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCreateWalletInvalidStatus)
		assert.Contains(t, err.Error(), "transaction execution error: wallet status is not pending for token")
	})

	t.Run("rolls back wallet update if TSS transaction creation fails", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		invalidService, err := NewEmbeddedWalletService(sdpModels, tssModel, "invalid_hash_not_32_bytes")
		require.NoError(t, err)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", data.PendingWalletStatus)
		walletIDForTest := initialWallet.Token

		err = invalidService.CreateWallet(ctx, walletIDForTest, defaultPublicKey, defaultCredentialID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating wallet transaction in TSS")

		unchangedWallet, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, walletIDForTest)
		require.NoError(t, err)
		assert.Equal(t, data.PendingWalletStatus, unchangedWallet.WalletStatus)
		assert.Empty(t, unchangedWallet.WasmHash)
	})

	t.Run("rolls back TSS transaction creation if wallet update fails", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		invalidService, err := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash)
		require.NoError(t, err)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", data.SuccessWalletStatus)
		walletIDForTest := initialWallet.Token

		err = invalidService.CreateWallet(ctx, walletIDForTest, defaultPublicKey, defaultCredentialID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transaction execution error: wallet status is not pending for token")

		tssTransactions, err := tssModel.GetAllByExternalIDs(ctx, []string{walletIDForTest})
		require.NoError(t, err)
		assert.Empty(t, tssTransactions)
	})

	t.Run("returns error when trying to create wallet with duplicate credential ID", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		duplicateCredentialID := "duplicate-credential-id"

		wallet1 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", data.PendingWalletStatus)
		err := service.CreateWallet(ctx, wallet1.Token, defaultPublicKey, duplicateCredentialID)
		require.NoError(t, err)

		wallet2 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", data.PendingWalletStatus)
		otherPublicKey := "deadbeef"
		err = service.CreateWallet(ctx, wallet2.Token, otherPublicKey, duplicateCredentialID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrCredentialIDAlreadyExists), "expected ErrCredentialIDAlreadyExists, got: %v", err)
	})
}

func Test_EmbeddedWalletService_GetWalletByCredentialID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	defaultTenantID := "test-tenant-id"
	ctx := tenant.SaveTenantInContext(context.Background(), &tenant.Tenant{ID: defaultTenantID})

	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tssModel := store.NewTransactionModel(dbConnectionPool)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service, err := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash)
	require.NoError(t, err)

	t.Run("successfully gets a wallet by credential ID", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		expectedWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "somehash", "somecontract", "test-credential-id", data.SuccessWalletStatus)

		retrievedWallet, err := service.GetWalletByCredentialID(ctx, expectedWallet.CredentialID)
		require.NoError(t, err)
		require.NotNil(t, retrievedWallet)

		assert.Equal(t, expectedWallet.Token, retrievedWallet.Token)
		assert.Equal(t, expectedWallet.WasmHash, retrievedWallet.WasmHash)
		assert.Equal(t, expectedWallet.ContractAddress, retrievedWallet.ContractAddress)
		assert.Equal(t, expectedWallet.CredentialID, retrievedWallet.CredentialID)
		assert.Equal(t, expectedWallet.WalletStatus, retrievedWallet.WalletStatus)
		assert.NotNil(t, retrievedWallet.CreatedAt)
		assert.NotNil(t, retrievedWallet.UpdatedAt)
	})

	t.Run("returns error if credential ID is empty", func(t *testing.T) {
		_, err := service.GetWalletByCredentialID(ctx, "")
		assert.EqualError(t, err, "credential ID is required")
	})

	t.Run("returns error if GetByCredentialID fails (wallet not found)", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
		nonExistentCredentialID := "non-existent-credential-id"
		_, err := service.GetWalletByCredentialID(ctx, nonExistentCredentialID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidCredentialID)
		assert.Contains(t, err.Error(), "credential ID does not exist")
	})
}
