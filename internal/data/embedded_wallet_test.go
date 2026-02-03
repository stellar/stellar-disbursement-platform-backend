package data

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_EmbeddedWalletColumnNames(t *testing.T) {
	testCases := []struct {
		tableReference string
		resultAlias    string
		expected       string
	}{
		{
			tableReference: "",
			resultAlias:    "",
			expected: strings.Join([]string{
				"token",
				"created_at",
				"updated_at",
				"wallet_status",
				"requires_verification",
				`COALESCE(wasm_hash, '') AS "wasm_hash"`,
				`COALESCE(contract_address, '') AS "contract_address"`,
				`COALESCE(credential_id, '') AS "credential_id"`,
				`COALESCE(public_key, '') AS "public_key"`,
				`COALESCE(receiver_wallet_id, '') AS "receiver_wallet_id"`,
			}, ", "),
		},
		{
			tableReference: "ew",
			resultAlias:    "",
			expected: strings.Join([]string{
				"ew.token",
				"ew.created_at",
				"ew.updated_at",
				"ew.wallet_status",
				"ew.requires_verification",
				`COALESCE(ew.wasm_hash, '') AS "wasm_hash"`,
				`COALESCE(ew.contract_address, '') AS "contract_address"`,
				`COALESCE(ew.credential_id, '') AS "credential_id"`,
				`COALESCE(ew.public_key, '') AS "public_key"`,
				`COALESCE(ew.receiver_wallet_id, '') AS "receiver_wallet_id"`,
			}, ", "),
		},
		{
			tableReference: "ew",
			resultAlias:    "embedded_wallets",
			expected: strings.Join([]string{
				`ew.token AS "embedded_wallets.token"`,
				`ew.created_at AS "embedded_wallets.created_at"`,
				`ew.updated_at AS "embedded_wallets.updated_at"`,
				`ew.wallet_status AS "embedded_wallets.wallet_status"`,
				`ew.requires_verification AS "embedded_wallets.requires_verification"`,
				`COALESCE(ew.wasm_hash, '') AS "embedded_wallets.wasm_hash"`,
				`COALESCE(ew.contract_address, '') AS "embedded_wallets.contract_address"`,
				`COALESCE(ew.credential_id, '') AS "embedded_wallets.credential_id"`,
				`COALESCE(ew.public_key, '') AS "embedded_wallets.public_key"`,
				`COALESCE(ew.receiver_wallet_id, '') AS "embedded_wallets.receiver_wallet_id"`,
			}, ", "),
		},
	}

	for _, tc := range testCases {
		t.Run(testCaseNameForScanText(t, tc.tableReference, tc.resultAlias), func(t *testing.T) {
			actual := EmbeddedWalletColumnNames(tc.tableReference, tc.resultAlias)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func Test_EmbeddedWalletModel_GetByToken(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	embeddedWalletModel := EmbeddedWalletModel{dbConnectionPool: dbConnectionPool}

	DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
	defer DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

	t.Run("returns error when wallet does not exist", func(t *testing.T) {
		wallet, err := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, "non_existent_token")
		require.Error(t, err)
		assert.Equal(t, ErrRecordNotFound, err)
		assert.Nil(t, wallet)
	})

	t.Run("returns wallet when it exists", func(t *testing.T) {
		expectedWasmHash := "abcdef123456"
		expectedContractAddress := "CDL5L3XOQRQMFL7J2W76GCQTFRRJUYEXXWGH32XC5UAT6X6H4K6XYZZA"
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", expectedWasmHash, expectedContractAddress, "", "", PendingWalletStatus)

		wallet, err := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet.Token)
		require.NoError(t, err)
		require.NotNil(t, wallet)

		assert.Equal(t, createdWallet.Token, wallet.Token)
		assert.Equal(t, expectedWasmHash, wallet.WasmHash)
		assert.Equal(t, expectedContractAddress, wallet.ContractAddress)
		assert.Equal(t, PendingWalletStatus, wallet.WalletStatus)
		assert.Equal(t, "", wallet.ReceiverWalletID)
		assert.False(t, wallet.RequiresVerification)
		assert.NotNil(t, wallet.CreatedAt)
		assert.NotNil(t, wallet.UpdatedAt)
	})
}

func Test_EmbeddedWalletUpdate_Validate(t *testing.T) {
	t.Run("returns error if no values provided", func(t *testing.T) {
		update := EmbeddedWalletUpdate{}
		err := update.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "no values provided to update embedded wallet")
	})

	t.Run("validates WasmHash", func(t *testing.T) {
		update := EmbeddedWalletUpdate{WasmHash: "invalid-hash"}
		err := update.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "invalid wasm hash")

		update = EmbeddedWalletUpdate{WasmHash: "abcdef123456"} // Valid hex
		err = update.Validate()
		require.NoError(t, err)
	})

	t.Run("validates ContractAddress", func(t *testing.T) {
		update := EmbeddedWalletUpdate{ContractAddress: "invalid-address"}
		err := update.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "invalid contract address")

		update = EmbeddedWalletUpdate{ContractAddress: "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"}
		err = update.Validate()
		require.NoError(t, err)
	})

	t.Run("validates WalletStatus", func(t *testing.T) {
		update := EmbeddedWalletUpdate{WalletStatus: "INVALID_STATUS"}
		err := update.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "validating wallet status: invalid embedded wallet status \"INVALID_STATUS\"")

		update = EmbeddedWalletUpdate{WalletStatus: SuccessWalletStatus}
		err = update.Validate()
		require.NoError(t, err)
	})

	t.Run("validates CredentialID", func(t *testing.T) {
		update := EmbeddedWalletUpdate{CredentialID: strings.Repeat("a", MaxCredentialIDLength+1)}
		err := update.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, fmt.Sprintf("credential ID must be %d characters or less, got %d characters", MaxCredentialIDLength, MaxCredentialIDLength+1))

		update = EmbeddedWalletUpdate{CredentialID: strings.Repeat("a", MaxCredentialIDLength)}
		err = update.Validate()
		require.NoError(t, err)
	})

	t.Run("validates a full valid update", func(t *testing.T) {
		update := EmbeddedWalletUpdate{
			WasmHash:        "abcdef123456",
			ContractAddress: "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53",
			WalletStatus:    SuccessWalletStatus,
		}
		err := update.Validate()
		require.NoError(t, err)
	})
}

func Test_EmbeddedWalletInsert_Validate(t *testing.T) {
	t.Run("returns error if token is empty", func(t *testing.T) {
		insert := EmbeddedWalletInsert{
			WalletStatus: PendingWalletStatus,
		}
		err := insert.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "token cannot be empty")
	})

	t.Run("returns error if wasm hash is empty", func(t *testing.T) {
		insert := EmbeddedWalletInsert{
			Token:        "token-123",
			WalletStatus: PendingWalletStatus,
		}
		err := insert.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "wasm hash cannot be empty")
	})

	t.Run("validates wasm hash when provided", func(t *testing.T) {
		insert := EmbeddedWalletInsert{
			Token:        "token-123",
			WasmHash:     "invalid-hash",
			WalletStatus: PendingWalletStatus,
		}
		err := insert.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "invalid wasm hash")
	})

	t.Run("validates wallet status", func(t *testing.T) {
		insert := EmbeddedWalletInsert{
			Token:        "token-123",
			WasmHash:     "abcdef123456",
			WalletStatus: "INVALID_STATUS",
		}
		err := insert.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "validating wallet status: invalid embedded wallet status \"INVALID_STATUS\"")

		insert.WalletStatus = SuccessWalletStatus
		err = insert.Validate()
		require.NoError(t, err)
	})

	t.Run("successfully validates all fields", func(t *testing.T) {
		insert := EmbeddedWalletInsert{
			Token:        "token-123",
			WasmHash:     "abcdef123456",
			WalletStatus: SuccessWalletStatus,
		}
		err := insert.Validate()
		require.NoError(t, err)
	})
}

func Test_EmbeddedWalletModel_Update(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	embeddedWalletModel := EmbeddedWalletModel{dbConnectionPool: dbConnectionPool}

	DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
	defer DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

	t.Run("returns error if update validation fails", func(t *testing.T) {
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "hash1", "contract1", "", "", PendingWalletStatus)
		invalidUpdate := EmbeddedWalletUpdate{WasmHash: "invalid"}
		err := embeddedWalletModel.Update(ctx, dbConnectionPool, createdWallet.Token, invalidUpdate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validating embedded wallet update: invalid wasm hash")
	})

	t.Run("returns error if wallet token does not exist", func(t *testing.T) {
		validUpdate := EmbeddedWalletUpdate{WalletStatus: SuccessWalletStatus}
		err := embeddedWalletModel.Update(ctx, dbConnectionPool, "non_existent_token", validUpdate)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Contains(t, err.Error(), "no embedded wallets could be found in UpdateEmbeddedWallet")
	})

	t.Run("successfully updates wallet status", func(t *testing.T) {
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "hash1", "contract1", "", "", PendingWalletStatus)
		update := EmbeddedWalletUpdate{WalletStatus: SuccessWalletStatus}

		err := embeddedWalletModel.Update(ctx, dbConnectionPool, createdWallet.Token, update)
		require.NoError(t, err)

		updatedWallet, getErr := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet.Token)
		require.NoError(t, getErr)
		assert.Equal(t, SuccessWalletStatus, updatedWallet.WalletStatus)
		assert.Equal(t, "hash1", updatedWallet.WasmHash) // Ensure other fields are not changed
		assert.Equal(t, "contract1", updatedWallet.ContractAddress)
	})

	t.Run("successfully updates wasm_hash and contract_address", func(t *testing.T) {
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", "", "", PendingWalletStatus)
		newWasmHash := "00112233aabbccdd"
		newContractAddress := "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"
		update := EmbeddedWalletUpdate{
			WasmHash:        newWasmHash,
			ContractAddress: newContractAddress,
		}

		err := embeddedWalletModel.Update(ctx, dbConnectionPool, createdWallet.Token, update)
		require.NoError(t, err)

		updatedWallet, getErr := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet.Token)
		require.NoError(t, getErr)
		assert.Equal(t, newWasmHash, updatedWallet.WasmHash)
		assert.Equal(t, newContractAddress, updatedWallet.ContractAddress)
		assert.Equal(t, PendingWalletStatus, updatedWallet.WalletStatus) // Ensure other fields are not changed
	})

	t.Run("successfully updates all fields", func(t *testing.T) {
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "old_hash", "old_contract", "", "", PendingWalletStatus)
		newWasmHash := "ddeeff0011223344"
		newContractAddress := "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53" // Placeholder
		newCredentialID := "new-credential-id"
		newPublicKey := "045666998adf1d157b9a3724f7c046b505e7f453026308f61681547ca315c6e120fd0b47ae80e8090197baf5be064b026365b0efae6a071f44841179a2937ca66e"
		newStatus := SuccessWalletStatus
		update := EmbeddedWalletUpdate{
			WasmHash:        newWasmHash,
			ContractAddress: newContractAddress,
			CredentialID:    newCredentialID,
			PublicKey:       newPublicKey,
			WalletStatus:    newStatus,
		}

		err := embeddedWalletModel.Update(ctx, dbConnectionPool, createdWallet.Token, update)
		require.NoError(t, err)

		updatedWallet, getErr := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet.Token)
		require.NoError(t, getErr)
		assert.Equal(t, newWasmHash, updatedWallet.WasmHash)
		assert.Equal(t, newContractAddress, updatedWallet.ContractAddress)
		assert.Equal(t, newCredentialID, updatedWallet.CredentialID)
		assert.Equal(t, newPublicKey, updatedWallet.PublicKey)
		assert.Equal(t, newStatus, updatedWallet.WalletStatus)
		assert.True(t, updatedWallet.UpdatedAt.After(*updatedWallet.CreatedAt))
	})

	t.Run("successfully toggles requires verification flag", func(t *testing.T) {
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "hash3", "contract3", "", "", PendingWalletStatus)
		update := EmbeddedWalletUpdate{
			RequiresVerification: utils.Ptr(true),
		}

		err := embeddedWalletModel.Update(ctx, dbConnectionPool, createdWallet.Token, update)
		require.NoError(t, err)

		updatedWallet, getErr := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet.Token)
		require.NoError(t, getErr)
		assert.True(t, updatedWallet.RequiresVerification)
	})

	t.Run("returns error when updating to duplicate credential_id", func(t *testing.T) {
		duplicateCredentialID := "duplicate-credential-id"

		createdWallet1 := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "hash1", "contract1", duplicateCredentialID, "", PendingWalletStatus)

		createdWallet2 := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "hash2", "contract2", "", "", PendingWalletStatus)

		update := EmbeddedWalletUpdate{CredentialID: duplicateCredentialID}
		err := embeddedWalletModel.Update(ctx, dbConnectionPool, createdWallet2.Token, update)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEmbeddedWalletCredentialIDAlreadyExists)

		unchangedWallet, getErr := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet1.Token)
		require.NoError(t, getErr)
		assert.Equal(t, duplicateCredentialID, unchangedWallet.CredentialID)

		unchangedWallet2, getErr := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet2.Token)
		require.NoError(t, getErr)
		assert.Equal(t, "", unchangedWallet2.CredentialID)
	})
}

func Test_EmbeddedWalletModel_GetPendingForSubmission(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	models, err := NewModels(dbConnectionPool)
	require.NoError(t, err)

	defer DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

	hash := strings.Repeat("a", 64)
	pubKey := strings.Repeat("b", 130)

	pending1 := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", hash, "", "cred-1", pubKey, PendingWalletStatus)
	pending2 := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", hash, "", "cred-2", pubKey, PendingWalletStatus)
	CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", hash, "", "cred-3", pubKey, SuccessWalletStatus)

	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer dbTx.Rollback()

	result, err := models.EmbeddedWallets.GetPendingForSubmission(ctx, dbTx, 5)
	require.NoError(t, err)
	require.Len(t, result, 2)

	ids := []string{result[0].Token, result[1].Token}
	assert.Contains(t, ids, pending1.Token)
	assert.Contains(t, ids, pending2.Token)
}

func Test_EmbeddedWalletModel_GetByReceiverWalletIDAndStatuses(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	embeddedWalletModel := EmbeddedWalletModel{dbConnectionPool: dbConnectionPool}

	DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
	defer DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "statuses-rw-wallet", "https://example.com", "wallet.example.com", "embedded://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

	t.Run("returns error for empty receiver wallet ID", func(t *testing.T) {
		result, err := embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, "", []EmbeddedWalletStatus{PendingWalletStatus})
		assert.ErrorIs(t, err, ErrMissingInput)
		assert.Nil(t, result)

		result, err = embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, "   ", []EmbeddedWalletStatus{PendingWalletStatus})
		assert.ErrorIs(t, err, ErrMissingInput)
		assert.Nil(t, result)
	})

	t.Run("returns error for empty statuses", func(t *testing.T) {
		result, err := embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, receiverWallet.ID, []EmbeddedWalletStatus{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one status must be provided")
		assert.Nil(t, result)
	})

	t.Run("returns ErrRecordNotFound when no matching wallet exists", func(t *testing.T) {
		result, err := embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, "non-existent-receiver-wallet-id", []EmbeddedWalletStatus{PendingWalletStatus})
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, result)
	})

	t.Run("returns wallet matching single status", func(t *testing.T) {
		DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		embedded := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "abcdef123456", "", "", "", PendingWalletStatus)
		require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, embedded.Token, EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

		result, err := embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, receiverWallet.ID, []EmbeddedWalletStatus{PendingWalletStatus})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, embedded.Token, result.Token)
		assert.Equal(t, PendingWalletStatus, result.WalletStatus)
	})

	t.Run("returns wallet matching one of multiple statuses", func(t *testing.T) {
		DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		embedded := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "abcdef123456", "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53", "cred-1", "pub-1", SuccessWalletStatus)
		require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, embedded.Token, EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

		result, err := embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, receiverWallet.ID, []EmbeddedWalletStatus{PendingWalletStatus, ProcessingWalletStatus, SuccessWalletStatus})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, embedded.Token, result.Token)
		assert.Equal(t, SuccessWalletStatus, result.WalletStatus)
	})

	t.Run("does not return wallet with non-matching status", func(t *testing.T) {
		DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		failedWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "abcdef123456", "", "cred-1", "pub-1", FailedWalletStatus)
		require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, failedWallet.Token, EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

		result, err := embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, receiverWallet.ID, []EmbeddedWalletStatus{PendingWalletStatus, ProcessingWalletStatus, SuccessWalletStatus})
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, result)
	})

	t.Run("returns matching wallet even when non-matching wallet also exists", func(t *testing.T) {
		DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		failedWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "failed-token", "abcdef123456", "", "cred-1", "pub-1", FailedWalletStatus)
		require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, failedWallet.Token, EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

		pendingWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "pending-token", "abcdef123456", "", "", "", PendingWalletStatus)
		require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, pendingWallet.Token, EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

		result, err := embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, receiverWallet.ID, []EmbeddedWalletStatus{PendingWalletStatus})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, pendingWallet.Token, result.Token)
		assert.Equal(t, PendingWalletStatus, result.WalletStatus)
	})

	t.Run("returns most recent wallet when multiple match", func(t *testing.T) {
		DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

		olderWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "older-token", "abcdef123456", "", "", "", PendingWalletStatus)
		require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, olderWallet.Token, EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

		newerWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "newer-token", "abcdef123456", "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53", "cred-1", "pub-1", SuccessWalletStatus)
		require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, newerWallet.Token, EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

		result, err := embeddedWalletModel.GetByReceiverWalletIDAndStatuses(ctx, dbConnectionPool, receiverWallet.ID, []EmbeddedWalletStatus{PendingWalletStatus, SuccessWalletStatus})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, newerWallet.Token, result.Token)
	})
}

func Test_EmbeddedWalletModel_GetReceiverWallet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	embeddedWalletModel := EmbeddedWalletModel{dbConnectionPool: dbConnectionPool}

	DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
	defer DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "token", "https://example.com", "wallet.example.com", "embedded://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
	contractAddress := "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX"
	embedded := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "token-2", "hash", contractAddress, "cred-id", "pub", PendingWalletStatus)
	update := EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}
	require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, embedded.Token, update))

	t.Run("success", func(t *testing.T) {
		wallet, err := embeddedWalletModel.GetReceiverWallet(ctx, dbConnectionPool, contractAddress)
		require.NoError(t, err)
		require.NotNil(t, wallet)
		assert.Equal(t, receiverWallet.ID, wallet.ID)
	})

	t.Run("not found", func(t *testing.T) {
		wallet, err := embeddedWalletModel.GetReceiverWallet(ctx, dbConnectionPool, "CDZMG22Z66UUW3Q7X7XZV3CNPAQWT7DAVBBFZTCTRAESJ5AZAVOMHFXC")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, wallet)
	})
}

func Test_EmbeddedWalletModel_GetPendingDisbursementAsset(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	embeddedWalletModel := EmbeddedWalletModel{dbConnectionPool: dbConnectionPool}

	DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)
	defer DeleteAllEmbeddedWalletsFixtures(t, ctx, dbConnectionPool)

	wallet := CreateWalletFixture(t, ctx, dbConnectionPool, "fixture-wallet", "https://example.com", "wallet.example.com", "embedded://")
	receiver := CreateReceiverFixture(t, ctx, dbConnectionPool, &Receiver{})
	receiverWallet := CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)
	asset := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	contractAddress := "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX"
	embedded := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "token-asset", "hash", contractAddress, "cred", "pub", PendingWalletStatus)
	require.NoError(t, embeddedWalletModel.Update(ctx, dbConnectionPool, embedded.Token, EmbeddedWalletUpdate{ReceiverWalletID: receiverWallet.ID}))

	disbursementModel := &DisbursementModel{dbConnectionPool: dbConnectionPool}
	disbursement := CreateDisbursementFixture(t, ctx, dbConnectionPool, disbursementModel, &Disbursement{
		Wallet: wallet,
		Asset:  asset,
		Status: StartedDisbursementStatus,
	})

	paymentModel := &PaymentModel{dbConnectionPool: dbConnectionPool}
	CreatePaymentFixture(t, ctx, dbConnectionPool, paymentModel, &Payment{
		ReceiverWallet: receiverWallet,
		Disbursement:   disbursement,
		Asset:          *asset,
		Status:         PendingPaymentStatus,
		Amount:         "15",
	})

	t.Run("success", func(t *testing.T) {
		result, err := embeddedWalletModel.GetPendingDisbursementAsset(ctx, dbConnectionPool, contractAddress)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, asset.ID, result.ID)
	})

	t.Run("not found", func(t *testing.T) {
		result, err := embeddedWalletModel.GetPendingDisbursementAsset(ctx, dbConnectionPool, "CDZMG22Z66UUW3Q7X7XZV3CNPAQWT7DAVBBFZTCTRAESJ5AZAVOMHFXC")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, result)
	})
}
