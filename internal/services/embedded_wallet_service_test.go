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
	recoveryAddress := "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB"

	t.Run("return an error if sdpModels is nil", func(t *testing.T) {
		tssModel := store.NewTransactionModel(dbConnectionPool)
		service, err := NewEmbeddedWalletService(nil, tssModel, wasmHash, recoveryAddress)
		assert.Nil(t, service)
		assert.EqualError(t, err, "sdpModels cannot be nil")
	})

	t.Run("return an error if tssModel is nil", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		service, err := NewEmbeddedWalletService(sdpModels, nil, wasmHash, recoveryAddress)
		assert.Nil(t, service)
		assert.EqualError(t, err, "tssModel cannot be nil")
	})

	t.Run("return an error if wasmHash is empty", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)
		tssModel := store.NewTransactionModel(dbConnectionPool)

		service, err := NewEmbeddedWalletService(sdpModels, tssModel, "", recoveryAddress)
		assert.Nil(t, service)
		assert.EqualError(t, err, "wasmHash cannot be empty")
	})

	t.Run("return an error if recoveryAddress is empty", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)
		tssModel := store.NewTransactionModel(dbConnectionPool)

		service, err := NewEmbeddedWalletService(sdpModels, tssModel, wasmHash, "")
		assert.Nil(t, service)
		assert.EqualError(t, err, "recoveryAddress cannot be empty")
	})

	t.Run("🎉 successfully creates a new EmbeddedWalletService instance", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)
		tssModel := store.NewTransactionModel(dbConnectionPool)

		service, err := NewEmbeddedWalletService(sdpModels, tssModel, wasmHash, recoveryAddress)
		require.NoError(t, err)
		require.NotNil(t, service)

		assert.Equal(t, sdpModels, service.sdpModels)
		assert.Equal(t, tssModel, service.tssModel)
		assert.Equal(t, wasmHash, service.wasmHash)
		assert.Equal(t, recoveryAddress, service.recoveryAddress)
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
	const testRecoveryAddress = "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB"

	service, err := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash, testRecoveryAddress)
	require.NoError(t, err)

	t.Run("successfully creates unique tokens", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)

		receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "test1@example.com",
		})
		receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			PhoneNumber: "+15551234567",
		})

		token1, err := service.CreateInvitationToken(ctx, "test1@example.com", "EMAIL", receiver1.ID)
		require.NoError(t, err)
		require.NotNil(t, token1)

		assert.NotEmpty(t, token1)

		wallet, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, token1)
		require.NoError(t, err)
		assert.Equal(t, token1, wallet.Token)
		assert.Equal(t, "test1@example.com", wallet.ReceiverContact)
		assert.Equal(t, data.ContactTypeEmail, wallet.ContactType)

		token2, err := service.CreateInvitationToken(ctx, "+15551234567", "PHONE_NUMBER", receiver2.ID)
		require.NoError(t, err)
		require.NotNil(t, token2)

		assert.NotEmpty(t, token2)

		wallet2, err := sdpModels.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, token2)
		require.NoError(t, err)
		assert.Equal(t, token2, wallet2.Token)
		assert.Equal(t, "+15551234567", wallet2.ReceiverContact)
		assert.Equal(t, data.ContactTypePhoneNumber, wallet2.ContactType)

		assert.NotEqual(t, token1, token2, "should generate unique tokens")
	})

	t.Run("returns error if receiver contact is empty", func(t *testing.T) {
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "test@example.com",
		})

		_, err := service.CreateInvitationToken(ctx, "", "EMAIL", receiver.ID)
		assert.EqualError(t, err, "receiver contact cannot be empty")
	})

	t.Run("returns error if contact type is empty", func(t *testing.T) {
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "test@example.com",
		})

		_, err := service.CreateInvitationToken(ctx, "test@example.com", "", receiver.ID)
		assert.EqualError(t, err, "contact type cannot be empty")
	})

	t.Run("returns error if contact type is invalid", func(t *testing.T) {
		defer data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{
			Email: "test@example.com",
		})

		_, err := service.CreateInvitationToken(ctx, "test@example.com", "INVALID", receiver.ID)
		assert.EqualError(t, err, "validating contact type: invalid contact type \"INVALID\"")
	})

	t.Run("returns error if receiver ID is empty", func(t *testing.T) {
		_, err := service.CreateInvitationToken(ctx, "test@example.com", "EMAIL", "")
		assert.EqualError(t, err, "receiver ID cannot be empty")
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
	const testRecoveryAddress = "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB"

	service, err := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash, testRecoveryAddress)
	require.NoError(t, err)

	defaultPublicKey := "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23"
	defaultCredentialID := "test-credential-id"

	t.Run("successfully creates a wallet TSS transaction", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test@example.com", "EMAIL", data.PendingWalletStatus)
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
		assert.NotEmpty(t, tssTransaction.WalletCreation.Salt, "salt should be generated from receiver contact")
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

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test@example.com", "EMAIL", data.SuccessWalletStatus)
		walletIDForTest := initialWallet.Token

		err := service.CreateWallet(ctx, walletIDForTest, defaultPublicKey, defaultCredentialID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCreateWalletInvalidStatus)
		assert.Contains(t, err.Error(), "transaction execution error: wallet status is not pending for token")
	})

	t.Run("rolls back wallet update if TSS transaction creation fails", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		invalidService, err := NewEmbeddedWalletService(sdpModels, tssModel, "invalid_hash_not_32_bytes", testRecoveryAddress)
		require.NoError(t, err)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test@example.com", "EMAIL", data.PendingWalletStatus)
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

		invalidService, err := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash, testRecoveryAddress)
		require.NoError(t, err)

		initialWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test@example.com", "EMAIL", data.SuccessWalletStatus)
		walletIDForTest := initialWallet.Token

		err = invalidService.CreateWallet(ctx, walletIDForTest, defaultPublicKey, defaultCredentialID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transaction execution error: wallet status is not pending for token")

		tssTransactions, err := tssModel.GetAllByExternalIDs(ctx, []string{walletIDForTest})
		require.NoError(t, err)
		assert.Empty(t, tssTransactions)
	})

	t.Run("generates consistent salt for same contact info", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		wallet1 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test@example.com", "EMAIL", data.PendingWalletStatus)
		wallet2 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test@example.com", "EMAIL", data.PendingWalletStatus)

		err := service.CreateWallet(ctx, wallet1.Token, defaultPublicKey, "cred1")
		require.NoError(t, err)
		err = service.CreateWallet(ctx, wallet2.Token, defaultPublicKey, "cred2")
		require.NoError(t, err)

		transactions1, err := tssModel.GetAllByExternalIDs(ctx, []string{wallet1.Token})
		require.NoError(t, err)
		require.Len(t, transactions1, 1)

		transactions2, err := tssModel.GetAllByExternalIDs(ctx, []string{wallet2.Token})
		require.NoError(t, err)
		require.Len(t, transactions2, 1)

		assert.Equal(t, transactions1[0].WalletCreation.Salt, transactions2[0].WalletCreation.Salt)
	})

	t.Run("generates different salt for different contact types", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		emailWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test@example.com", "EMAIL", data.PendingWalletStatus)
		phoneWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "5551234567", "PHONE_NUMBER", data.PendingWalletStatus)

		err := service.CreateWallet(ctx, emailWallet.Token, defaultPublicKey, "cred1")
		require.NoError(t, err)
		err = service.CreateWallet(ctx, phoneWallet.Token, defaultPublicKey, "cred2")
		require.NoError(t, err)

		emailTransactions, err := tssModel.GetAllByExternalIDs(ctx, []string{emailWallet.Token})
		require.NoError(t, err)
		require.Len(t, emailTransactions, 1)

		phoneTransactions, err := tssModel.GetAllByExternalIDs(ctx, []string{phoneWallet.Token})
		require.NoError(t, err)
		require.Len(t, phoneTransactions, 1)

		assert.NotEqual(t, emailTransactions[0].WalletCreation.Salt, phoneTransactions[0].WalletCreation.Salt)
	})

	t.Run("returns error when trying to create wallet with duplicate credential ID", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		duplicateCredentialID := "duplicate-credential-id"

		wallet1 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test1@example.com", "EMAIL", data.PendingWalletStatus)
		err := service.CreateWallet(ctx, wallet1.Token, defaultPublicKey, duplicateCredentialID)
		require.NoError(t, err)

		wallet2 := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "test2@example.com", "EMAIL", data.PendingWalletStatus)
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
	const testRecoveryAddress = "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB"

	service, err := NewEmbeddedWalletService(sdpModels, tssModel, testWasmHash, testRecoveryAddress)
	require.NoError(t, err)

	t.Run("successfully gets a wallet by credential ID", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		expectedWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "somehash", "somecontract", "test-credential-id", "test@example.com", "EMAIL", data.SuccessWalletStatus)

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

func Test_EmbeddedWalletService_GetWalletByToken(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	tssModel := store.NewTransactionModel(dbConnectionPool)

	service, err := NewEmbeddedWalletService(sdpModels, tssModel, "somehash", "GBYJZW5XFAI6XV73H5SAIUYK6XZI4CGGVBUBO3ANA2SV7KKDAXTV6AEB")
	require.NoError(t, err)

	ctx := tenant.SaveTenantInContext(context.Background(), &tenant.Tenant{ID: "tenant-id"})

	t.Run("successfully gets a wallet by token", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		expectedWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "somehash", "somecontract", "test-credential-id", "test@example.com", "EMAIL", data.SuccessWalletStatus)

		retrievedWallet, err := service.GetWalletByToken(ctx, expectedWallet.Token)
		require.NoError(t, err)
		require.NotNil(t, retrievedWallet)

		assert.Equal(t, expectedWallet.Token, retrievedWallet.Token)
		assert.Equal(t, expectedWallet.WasmHash, retrievedWallet.WasmHash)
		assert.Equal(t, expectedWallet.ContractAddress, retrievedWallet.ContractAddress)
		assert.Equal(t, expectedWallet.CredentialID, retrievedWallet.CredentialID)
		assert.Equal(t, expectedWallet.ReceiverContact, retrievedWallet.ReceiverContact)
		assert.Equal(t, expectedWallet.ContactType, retrievedWallet.ContactType)
		assert.Equal(t, expectedWallet.WalletStatus, retrievedWallet.WalletStatus)
		assert.NotNil(t, retrievedWallet.CreatedAt)
		assert.NotNil(t, retrievedWallet.UpdatedAt)
	})

	t.Run("returns error if token is empty", func(t *testing.T) {
		_, err := service.GetWalletByToken(ctx, "")
		assert.ErrorIs(t, err, ErrInvalidToken)
	})

	t.Run("returns error if GetByToken fails (wallet not found)", func(t *testing.T) {
		defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
		nonExistentToken := "non-existent-token"
		_, err := service.GetWalletByToken(ctx, nonExistentToken)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidToken)
	})
}
