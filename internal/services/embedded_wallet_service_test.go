package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	stellarMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/stellar/mocks"
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
		service, err := NewEmbeddedWalletService(nil, wasmHash, stellarMocks.NewMockRPCClient(t))
		assert.Nil(t, service)
		assert.EqualError(t, err, "sdpModels cannot be nil")
	})

	t.Run("return an error if wasmHash is empty", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		service, err := NewEmbeddedWalletService(sdpModels, "", stellarMocks.NewMockRPCClient(t))
		assert.Nil(t, service)
		assert.EqualError(t, err, "wasmHash cannot be empty")
	})

	t.Run("return an error if rpc client is nil", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		service, err := NewEmbeddedWalletService(sdpModels, wasmHash, nil)
		assert.Nil(t, service)
		assert.EqualError(t, err, "rpcClient cannot be nil")
	})

	t.Run("🎉 successfully creates a new EmbeddedWalletService instance", func(t *testing.T) {
		sdpModels, err := data.NewModels(dbConnectionPool)
		require.NoError(t, err)

		service, err := NewEmbeddedWalletService(sdpModels, wasmHash, stellarMocks.NewMockRPCClient(t))
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

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, stellarMocks.NewMockRPCClient(t))
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

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, stellarMocks.NewMockRPCClient(t))
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

func Test_EmbeddedWalletService_SponsorTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"
	rpcMock := stellarMocks.NewMockRPCClient(t)
	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, rpcMock)
	require.NoError(t, err)

	const contractAddress = "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX"
	const distributionAccount = "GCLWGQPMKXQSPF776IU33AH4PZNOOWNAWGGKVTBQMIC5IMKUNP3E6NVU"
	const operationXDR = "AAAAAAAAAAHXkotywnA8z+r365/0701QSlWouXn8m0UOoshCtNHOYQAAAAh0cmFuc2ZlcgAAAAAAAAAA"

	data.DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

	errorCases := []struct {
		name         string
		account      string
		operationXDR string
		expectedErr  error
	}{
		{
			name:         "returns error when account is empty",
			account:      "",
			operationXDR: operationXDR,
			expectedErr:  ErrMissingAccount,
		},
		{
			name:         "returns error when operation XDR is empty",
			account:      contractAddress,
			operationXDR: "",
			expectedErr:  ErrMissingOperationXDR,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.SponsorTransaction(ctx, tc.account, tc.operationXDR)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}

	t.Run("returns error when simulation fails", func(t *testing.T) {
		rpcMock.
			On("SimulateTransaction", mock.Anything, mock.MatchedBy(func(req protocol.SimulateTransactionRequest) bool {
				return req.AuthMode == protocol.AuthModeEnforce
			})).
			Return((*stellar.SimulationResult)(nil), stellar.NewSimulationError(errors.New("boom"), nil)).
			Once()

		ctxWithTenant := sdpcontext.SetTenantInContext(ctx, &schema.Tenant{
			DistributionAccountAddress: func() *string {
				address := distributionAccount
				return &address
			}(),
		})

		_, err := service.SponsorTransaction(ctxWithTenant, contractAddress, operationXDR)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "simulating sponsored transaction")
	})

	t.Run("successfully creates sponsored transaction for embedded wallet", func(t *testing.T) {
		ctxWithTenant := sdpcontext.SetTenantInContext(ctx, &schema.Tenant{
			DistributionAccountAddress: func() *string {
				address := distributionAccount
				return &address
			}(),
		})

		rpcMock.
			On("SimulateTransaction", mock.Anything, mock.MatchedBy(func(req protocol.SimulateTransactionRequest) bool {
				return req.AuthMode == protocol.AuthModeEnforce
			})).
			Return(&stellar.SimulationResult{Response: protocol.SimulateTransactionResponse{}}, (*stellar.SimulationError)(nil)).
			Once()

		data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", testWasmHash, contractAddress, "credential", "public-key", data.SuccessWalletStatus)

		transactionID, err := service.SponsorTransaction(ctxWithTenant, contractAddress, operationXDR)
		require.NoError(t, err)
		require.NotEmpty(t, transactionID)

		transaction, err := sdpModels.SponsoredTransactions.GetByID(ctx, dbConnectionPool, transactionID)
		require.NoError(t, err)
		assert.Equal(t, contractAddress, transaction.Account)
		assert.Equal(t, operationXDR, transaction.OperationXDR)
	})
}

func Test_EmbeddedWalletService_GetTransactionStatus(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"
	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, stellarMocks.NewMockRPCClient(t))
	require.NoError(t, err)

	const contractAddress = "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX"
	const otherAddress = "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4"

	data.DeleteAllSponsoredTransactionsFixtures(t, ctx, dbConnectionPool)

	t.Run("returns error when account is empty", func(t *testing.T) {
		_, err := service.GetTransactionStatus(ctx, "", "transaction-id")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingAccount)
	})

	t.Run("returns error when transaction ID is empty", func(t *testing.T) {
		_, err := service.GetTransactionStatus(ctx, contractAddress, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transaction ID is required")
	})

	t.Run("returns not found when transaction does not belong to account", func(t *testing.T) {
		transaction := data.CreateSponsoredTransactionFixture(t, ctx, dbConnectionPool, contractAddress, "operation-xdr")

		_, err := service.GetTransactionStatus(ctx, otherAddress, transaction.ID)
		require.Error(t, err)
		assert.ErrorIs(t, err, data.ErrRecordNotFound)
	})

	t.Run("returns sponsored transaction when account matches", func(t *testing.T) {
		transaction := data.CreateSponsoredTransactionFixture(t, ctx, dbConnectionPool, contractAddress, "operation-xdr")

		found, err := service.GetTransactionStatus(ctx, contractAddress, transaction.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, transaction.ID, found.ID)
		assert.Equal(t, contractAddress, found.Account)
	})
}

func Test_EmbeddedWalletService_GetPendingDisbursementAsset(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"
	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, stellarMocks.NewMockRPCClient(t))
	require.NoError(t, err)

	contractAddress := "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4"
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "ew-refresh", "https://example.com", "ew-refresh.stellar", "embedded://")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, sdpModels.Disbursements, &data.Disbursement{
		Wallet: wallet,
		Asset:  asset,
		Status: data.StartedDisbursementStatus,
	})

	embeddedWallet := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", testWasmHash, "", "credential", "public-key", data.SuccessWalletStatus)
	update := data.EmbeddedWalletUpdate{
		ReceiverWalletID: receiverWallet.ID,
		ContractAddress:  contractAddress,
	}
	require.NoError(t, sdpModels.EmbeddedWallets.Update(ctx, dbConnectionPool, embeddedWallet.Token, update))

	_ = data.CreatePaymentFixture(t, ctx, dbConnectionPool, sdpModels.Payment, &data.Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Status:         data.PendingPaymentStatus,
		Amount:         "15",
	})

	resultAsset, err := service.GetPendingDisbursementAsset(ctx, contractAddress)
	require.NoError(t, err)
	require.NotNil(t, resultAsset)
	assert.Equal(t, asset.ID, resultAsset.ID)
	assert.Equal(t, asset.Code, resultAsset.Code)
	assert.Equal(t, asset.Issuer, resultAsset.Issuer)

	t.Run("returns nil when no pending asset exists", func(t *testing.T) {
		assetResult, lookupErr := service.GetPendingDisbursementAsset(ctx, "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMZZ")
		require.NoError(t, lookupErr)
		assert.Nil(t, assetResult)
	})

	t.Run("returns error when contract address empty", func(t *testing.T) {
		assetResult, lookupErr := service.GetPendingDisbursementAsset(ctx, "")
		require.Error(t, lookupErr)
		assert.Nil(t, assetResult)
		assert.ErrorIs(t, lookupErr, ErrMissingContractAddress)
	})
}

func Test_EmbeddedWalletService_getReceiverWalletByContractAddress(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, stellarMocks.NewMockRPCClient(t))
	require.NoError(t, err)

	data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
	defer data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "token", "https://example.com", "wallet.example.com", "embedded://")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
	contractAddress := "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R4"
	embedded := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "token-2", testWasmHash, contractAddress, "cred", "pub", data.PendingWalletStatus)
	require.NoError(t, sdpModels.EmbeddedWallets.Update(ctx, dbConnectionPool, embedded.Token, data.EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

	t.Run("success", func(t *testing.T) {
		wallet, err := service.getReceiverWalletByContractAddress(ctx, contractAddress)
		require.NoError(t, err)
		require.NotNil(t, wallet)
		assert.Equal(t, receiverWallet.ID, wallet.ID)
	})

	t.Run("not found", func(t *testing.T) {
		wallet, err := service.getReceiverWalletByContractAddress(ctx, "CDZMG22Z66UUW3Q7X7XZV3CNPAQWT7DAVBBFZTCTRAESJ5AZAVOMHFXC")
		require.Error(t, err)
		assert.ErrorIs(t, err, data.ErrRecordNotFound)
		assert.Nil(t, wallet)
	})
}

func Test_EmbeddedWalletService_IsVerificationPending(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, stellarMocks.NewMockRPCClient(t))
	require.NoError(t, err)

	cleanupFixtures := func(t *testing.T) {
		data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
		data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
	}
	cleanupFixtures(t)

	setupReceiverWallet := func(t *testing.T, status data.ReceiversWalletStatus, contractAddress string) {
		cleanupFixtures(t)

		wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "wallet-"+string(status), "https://example.com", "wallet.example.com", "embedded://")
		receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, status)
		embedded := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "token-"+string(status), testWasmHash, contractAddress, "cred-"+string(status), "pub-"+string(status), data.PendingWalletStatus)
		require.NoError(t, sdpModels.EmbeddedWallets.Update(ctx, dbConnectionPool, embedded.Token, data.EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))
	}

	t.Run("returns error when contract address empty", func(t *testing.T) {
		isPending, err := service.IsVerificationPending(ctx, "   ")
		require.Error(t, err)
		assert.False(t, isPending)
		assert.ErrorIs(t, err, ErrMissingContractAddress)
	})

	t.Run("returns error when receiver wallet missing", func(t *testing.T) {
		_, err := service.IsVerificationPending(ctx, "CDZMG22Z66UUW3Q7X7XZV3CNPAQWT7DAVBBFZTCTRAESJ5AZAVOMHFXC")
		require.Error(t, err)
		assert.ErrorIs(t, err, data.ErrRecordNotFound)
	})

	t.Run("returns true when receiver wallet ready", func(t *testing.T) {
		setupReceiverWallet(t, data.ReadyReceiversWalletStatus, "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX")

		isPending, err := service.IsVerificationPending(ctx, "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX")
		require.NoError(t, err)
		assert.True(t, isPending)
	})

	t.Run("returns false when receiver wallet registered", func(t *testing.T) {
		setupReceiverWallet(t, data.RegisteredReceiversWalletStatus, "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R5")

		isPending, err := service.IsVerificationPending(ctx, "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R5")
		require.NoError(t, err)
		assert.False(t, isPending)
	})
}

func Test_EmbeddedWalletService_GetReceiverContact(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	sdpModels, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	const testWasmHash = "e5da3b9950524b4276ccf2051e6cc8220bb581e869b892a6ff7812d7709c7a50"

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, stellarMocks.NewMockRPCClient(t))
	require.NoError(t, err)

	data.DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllReceiverWalletsFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllReceiversFixtures(t, ctx, dbConnectionPool)
	data.DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "contact-wallet", "https://example.com", "wallet.example.com", "embedded://")
	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{Email: "test@example.com", PhoneNumber: "+123456"})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
	contractAddress := "CCYU2FUIMK23K34U3SWCN2O2JVI6JBGUGQUILYK7GRPCIDABVVTCS7R9"
	embedded := data.CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "token-contact", testWasmHash, contractAddress, "cred-contact", "pub-contact", data.PendingWalletStatus)
	require.NoError(t, sdpModels.EmbeddedWallets.Update(ctx, dbConnectionPool, embedded.Token, data.EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

	t.Run("success", func(t *testing.T) {
		contact, err := service.GetReceiverContact(ctx, contractAddress)
		require.NoError(t, err)
		require.NotNil(t, contact)
		assert.Equal(t, receiver.Email, contact.Email)
		assert.Equal(t, receiver.PhoneNumber, contact.PhoneNumber)
	})

	t.Run("returns error when contract address empty", func(t *testing.T) {
		contact, err := service.GetReceiverContact(ctx, "")
		require.Error(t, err)
		assert.Nil(t, contact)
		assert.ErrorIs(t, err, ErrMissingContractAddress)
	})

	t.Run("returns error when receiver wallet missing", func(t *testing.T) {
		contact, err := service.GetReceiverContact(ctx, "CDZMG22Z66UUW3Q7X7XZV3CNPAQWT7DAVBBFZTCTRAESJ5AZAVOMHFXC")
		require.Error(t, err)
		assert.Nil(t, contact)
		assert.ErrorIs(t, err, data.ErrRecordNotFound)
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

	service, err := NewEmbeddedWalletService(sdpModels, testWasmHash, stellarMocks.NewMockRPCClient(t))
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
