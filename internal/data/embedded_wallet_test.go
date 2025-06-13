package data

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
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
				`COALESCE(wasm_hash, '') AS "wasm_hash"`,
				`COALESCE(contract_address, '') AS "contract_address"`,
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
				`COALESCE(ew.wasm_hash, '') AS "wasm_hash"`,
				`COALESCE(ew.contract_address, '') AS "contract_address"`,
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
				`COALESCE(ew.wasm_hash, '') AS "embedded_wallets.wasm_hash"`,
				`COALESCE(ew.contract_address, '') AS "embedded_wallets.contract_address"`,
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
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", expectedWasmHash, expectedContractAddress, PendingWalletStatus)

		wallet, err := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet.Token)
		require.NoError(t, err)
		require.NotNil(t, wallet)

		assert.Equal(t, createdWallet.Token, wallet.Token)
		assert.Equal(t, expectedWasmHash, wallet.WasmHash)
		assert.Equal(t, expectedContractAddress, wallet.ContractAddress)
		assert.Equal(t, PendingWalletStatus, wallet.WalletStatus)
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
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "hash1", "contract1", PendingWalletStatus)
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
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "hash1", "contract1", PendingWalletStatus)
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
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "", "", PendingWalletStatus)
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
		createdWallet := CreateEmbeddedWalletFixture(t, ctx, dbConnectionPool, "", "old_hash", "old_contract", PendingWalletStatus)
		newWasmHash := "ddeeff0011223344"
		newContractAddress := "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53" // Placeholder
		newStatus := SuccessWalletStatus
		update := EmbeddedWalletUpdate{
			WasmHash:        newWasmHash,
			ContractAddress: newContractAddress,
			WalletStatus:    newStatus,
		}

		err := embeddedWalletModel.Update(ctx, dbConnectionPool, createdWallet.Token, update)
		require.NoError(t, err)

		updatedWallet, getErr := embeddedWalletModel.GetByToken(ctx, dbConnectionPool, createdWallet.Token)
		require.NoError(t, getErr)
		assert.Equal(t, newWasmHash, updatedWallet.WasmHash)
		assert.Equal(t, newContractAddress, updatedWallet.ContractAddress)
		assert.Equal(t, newStatus, updatedWallet.WalletStatus)
		assert.True(t, updatedWallet.UpdatedAt.After(*updatedWallet.CreatedAt))
	})
}
