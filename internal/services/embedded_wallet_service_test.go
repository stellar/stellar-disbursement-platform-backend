package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func Test_NewEmbeddedWalletService(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	wasmHash := "e5da3b"

	t.Run("return an error if sdpModels is nil", func(t *testing.T) {
		service, err := NewEmbeddedWalletService(nil, wasmHash)
		assert.Nil(t, service)
		assert.EqualError(t, err, "sdpModels cannot be nil")
	})

	t.Run("return an error if wasmHash is empty", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		service, err := NewEmbeddedWalletService(sdpModels, "")
		assert.Nil(t, service)
		assert.EqualError(t, err, "wasmHash cannot be empty")
	})

	t.Run("🎉 successfully creates a new EmbeddedWalletService instance", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		service, err := NewEmbeddedWalletService(sdpModels, wasmHash)
		require.NoError(t, err)
		require.NotNil(t, service)

		assert.Equal(t, sdpModels, service.sdpModels)
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
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash)
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

	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: "test-tenant-id"})

	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash)
	require.NoError(t, err)

	defaultPublicKey := "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23"
	defaultCredentialID := "test-credential-id"

	t.Run("updates wallet fields and keeps status pending", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)
		walletIDForTest := initialWallet.Token

		err := service.CreateWallet(ctx, walletIDForTest, defaultPublicKey, defaultCredentialID)
		require.NoError(t, err)

		updatedWallet, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, walletIDForTest)
		require.NoError(t, err)
		require.NotNil(t, updatedWallet)
		assert.Equal(t, data.PendingWalletStatus, updatedWallet.WalletStatus)
		assert.Equal(t, testWasmHash, updatedWallet.WasmHash)
		assert.Equal(t, defaultCredentialID, updatedWallet.CredentialID)
		assert.Equal(t, defaultPublicKey, updatedWallet.PublicKey)
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
		assert.Contains(t, err.Error(), "token does not exist")
	})

	t.Run("returns error if wallet status is not pending", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.SuccessWalletStatus)
		walletIDForTest := initialWallet.Token

		err := service.CreateWallet(ctx, walletIDForTest, defaultPublicKey, defaultCredentialID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCreateWalletInvalidStatus)
		assert.Contains(t, err.Error(), "wallet status is not pending for token")
	})

	t.Run("returns error when trying to create wallet with duplicate credential ID", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		duplicateCredentialID := "duplicate-credential-id"

		// Step 1: Create first wallet successfully
		wallet1 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)
		err := service.CreateWallet(ctx, wallet1.Token, defaultPublicKey, duplicateCredentialID)
		require.NoError(t, err)

		updatedWallet1, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, wallet1.Token)
		require.NoError(t, err)
		assert.Equal(t, duplicateCredentialID, updatedWallet1.CredentialID)
		assert.Equal(t, data.PendingWalletStatus, updatedWallet1.WalletStatus)

		// Step 2: Try duplicate credential ID (should fail cleanly)
		wallet2 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)
		otherPublicKey := "04deadbeef" + strings.Repeat("00", 60)
		err = service.CreateWallet(ctx, wallet2.Token, otherPublicKey, duplicateCredentialID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrCredentialIDAlreadyExists))

		// Verify second wallet remains unchanged
		unchangedWallet2, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, wallet2.Token)
		require.NoError(t, err)
		assert.Empty(t, unchangedWallet2.CredentialID)
		assert.Equal(t, data.PendingWalletStatus, unchangedWallet2.WalletStatus)
	})

	t.Run("allows retry with different credential ID after failure", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		firstCredentialID := "first-credential-id"
		secondCredentialID := "second-credential-id"

		// Step 1: Create first wallet successfully
		wallet1 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)
		err := service.CreateWallet(ctx, wallet1.Token, defaultPublicKey, firstCredentialID)
		require.NoError(t, err)

		updatedWallet1, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, wallet1.Token)
		require.NoError(t, err)
		assert.Equal(t, data.PendingWalletStatus, updatedWallet1.WalletStatus)

		// Step 2: Try duplicate credential ID (should fail)
		wallet2 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", data.PendingWalletStatus)
		otherPublicKey := "04deadbeef" + strings.Repeat("00", 60)
		err = service.CreateWallet(ctx, wallet2.Token, otherPublicKey, firstCredentialID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrCredentialIDAlreadyExists))

		// Step 3: Retry with different credential ID (should succeed)
		err = service.CreateWallet(ctx, wallet2.Token, otherPublicKey, secondCredentialID)
		require.NoError(t, err)

		// Verify second wallet was updated successfully
		updatedWallet2, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, wallet2.Token)
		require.NoError(t, err)
		assert.Equal(t, secondCredentialID, updatedWallet2.CredentialID)
		assert.Equal(t, data.PendingWalletStatus, updatedWallet2.WalletStatus)
	})
}

func Test_EmbeddedWalletService_GetWalletByCredentialID(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	defaultTenantID := "test-tenant-id"
	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: defaultTenantID})

	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash)
	require.NoError(t, err)

	t.Run("successfully gets a wallet by credential ID", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		expectedWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "somehash", "somecontract", "test-credential-id", "test-public-key", data.SuccessWalletStatus)

		retrievedWallet, err := service.GetWalletByCredentialID(ctx, expectedWallet.CredentialID)
		require.NoError(t, err)
		require.NotNil(t, retrievedWallet)

		assert.Equal(t, expectedWallet.Token, retrievedWallet.Token)
		assert.Equal(t, expectedWallet.WasmHash, retrievedWallet.WasmHash)
		assert.Equal(t, expectedWallet.ContractAddress, retrievedWallet.ContractAddress)
		assert.Equal(t, expectedWallet.CredentialID, retrievedWallet.CredentialID)
		assert.Equal(t, expectedWallet.PublicKey, retrievedWallet.PublicKey)
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
