package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stellar/go/strkey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

const (
	testWasmHash            = "a5016f845e76fe452de6d3638ac47523b845a813db56de3d713eb7a49276e254"
	testPublicKey           = "04f5549c5ef833ab0ade80d9c1f3fb34fb93092503a8ce105773d676288653df384a024a92cc73cb8089c45ed76ed073433b6a72c64a6ed23630b77327beb65f23"
	testDistributionAccount = "GCLWGQPMKXQSPF776IU33AH4PZNOOWNAWGGKVTBQMIC5IMKUNP3E6NVU"
	testNetworkPassphrase   = "Test SDF Network ; September 2015"
)

func setupEmbeddedWalletTestContext(t *testing.T, dbConnectionPool db.DBConnectionPool) *testContext {
	t.Helper()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	tssModel := txSubStore.NewTransactionModel(models.DBConnectionPool)

	return &testContext{
		tssModel: tssModel,
		sdpModel: models,
		ctx:      context.Background(),
		tenantID: uuid.NewString(),
	}
}

func Test_WalletCreationFromSubmitterService_SyncTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupEmbeddedWalletTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	service := NewWalletCreationFromSubmitterService(testCtx.sdpModel, dbConnectionPool, testNetworkPassphrase)

	walletToken := uuid.NewString()
	_, err := testCtx.sdpModel.EmbeddedWallets.Insert(ctx, dbConnectionPool, data.EmbeddedWalletInsert{
		Token:        walletToken,
		WasmHash:     testWasmHash,
		WalletStatus: data.ProcessingWalletStatus,
	})
	require.NoError(t, err)

	t.Run("successfully syncs successful wallet creation transaction", func(t *testing.T) {
		// Create successful wallet creation transaction
		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      walletToken,
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'success_hash_123', status=$1, distribution_account=$2 WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, txSubStore.TransactionStatusProcessing, testDistributionAccount, tssTransaction.ID)
		require.NoError(t, err)

		tssTransaction, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tssTransaction)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		require.NoError(t, err)

		updatedWallet, err := testCtx.sdpModel.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, walletToken)
		require.NoError(t, err)
		assert.Equal(t, data.SuccessWalletStatus, updatedWallet.WalletStatus)
		assert.NotEmpty(t, updatedWallet.ContractAddress)

		// Verify transaction was marked as synced
		_, err = testCtx.tssModel.GetTransactionPendingUpdateByID(ctx, dbConnectionPool, tssTransaction.ID, txSubStore.TransactionTypeWalletCreation)
		assert.ErrorIs(t, err, txSubStore.ErrRecordNotFound, "transaction should be marked as synced")
	})

	t.Run("successfully syncs failed wallet creation transaction", func(t *testing.T) {
		// Create another embedded wallet for failed test
		failedWalletToken := uuid.NewString()
		_, err := testCtx.sdpModel.EmbeddedWallets.Insert(ctx, dbConnectionPool, data.EmbeddedWalletInsert{
			Token:        failedWalletToken,
			WasmHash:     testWasmHash,
			WalletStatus: data.ProcessingWalletStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      failedWalletToken,
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'error_hash_123', status=$1, status_message=$2, distribution_account=$3 WHERE id = $4 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, txSubStore.TransactionStatusError, "Transaction failed", testDistributionAccount, tssTransaction.ID)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		require.NoError(t, err)

		updatedWallet, err := testCtx.sdpModel.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, failedWalletToken)
		require.NoError(t, err)
		assert.Equal(t, data.FailedWalletStatus, updatedWallet.WalletStatus)
		assert.Empty(t, updatedWallet.ContractAddress)

		_, err = testCtx.tssModel.GetTransactionPendingUpdateByID(ctx, dbConnectionPool, tssTransaction.ID, txSubStore.TransactionTypeWalletCreation)
		assert.ErrorIs(t, err, txSubStore.ErrRecordNotFound, "transaction should be marked as synced")
	})

	t.Run("successfully syncs failed wallet creation transaction (RPC simulation failure)", func(t *testing.T) {
		// Create embedded wallet for RPC simulation failure test
		rpcFailWalletToken := uuid.NewString()
		_, err := testCtx.sdpModel.EmbeddedWallets.Insert(ctx, dbConnectionPool, data.EmbeddedWalletInsert{
			Token:        rpcFailWalletToken,
			WasmHash:     testWasmHash,
			WalletStatus: data.ProcessingWalletStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      rpcFailWalletToken,
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		// Simulate RPC simulation failure by updating to ERROR without stellar transaction hash or distribution account
		q := `UPDATE submitter_transactions SET status=$1, status_message=$2, completed_at=NOW() WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, txSubStore.TransactionStatusError, "RPC simulation failed: contract already exists", tssTransaction.ID)
		require.NoError(t, err)

		assert.False(t, tssTransaction.StellarTransactionHash.Valid)
		assert.False(t, tssTransaction.DistributionAccount.Valid)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		require.NoError(t, err)

		updatedWallet, err := testCtx.sdpModel.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, rpcFailWalletToken)
		require.NoError(t, err)
		assert.Equal(t, data.FailedWalletStatus, updatedWallet.WalletStatus)
		assert.Empty(t, updatedWallet.ContractAddress)

		_, err = testCtx.tssModel.GetTransactionPendingUpdateByID(ctx, dbConnectionPool, tssTransaction.ID, txSubStore.TransactionTypeWalletCreation)
		assert.ErrorIs(t, err, txSubStore.ErrRecordNotFound, "transaction should be marked as synced")
	})
}

func Test_WalletCreationFromSubmitterService_SyncTransaction_errors(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupEmbeddedWalletTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	service := NewWalletCreationFromSubmitterService(testCtx.sdpModel, dbConnectionPool, testNetworkPassphrase)

	t.Run("returns error for non-existent transaction", func(t *testing.T) {
		err := service.SyncTransaction(ctx, "non-existent-tx-id")
		assert.ErrorContains(t, err, "wallet creation transaction non-existent-tx-id not found or wrong type")
	})

	t.Run("returns error for non-wallet-creation transaction", func(t *testing.T) {
		paymentTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      "payment-id-123",
			TransactionType: txSubStore.TransactionTypePayment,
			Payment: txSubStore.Payment{
				AssetCode:   "USDC",
				AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				Amount:      100,
				Destination: "GCLWGQPMKXQSPF776IU33AH4PZNOOWNAWGGKVTBQMIC5IMKUNP3E6NVU",
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'payment_hash_123', status=$1 WHERE id = $2 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, paymentTransaction, q, txSubStore.TransactionStatusProcessing, paymentTransaction.ID)
		require.NoError(t, err)

		paymentTransaction, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *paymentTransaction)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, paymentTransaction.ID)
		assert.ErrorContains(t, err, "wallet creation transaction")
		assert.ErrorContains(t, err, "not found or wrong type")
	})

	t.Run("returns error for non-existent embedded wallet", func(t *testing.T) {
		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      "non-existent-wallet-token",
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'wallet_hash_123', status=$1, distribution_account=$2 WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, txSubStore.TransactionStatusProcessing, testDistributionAccount, tssTransaction.ID)
		require.NoError(t, err)

		tssTransaction, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tssTransaction)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		assert.ErrorContains(t, err, "embedded wallet with token non-existent-wallet-token not found")
	})

	t.Run("returns error when transaction is not in terminal state", func(t *testing.T) {
		walletToken := uuid.NewString()
		_, err := testCtx.sdpModel.EmbeddedWallets.Insert(ctx, dbConnectionPool, data.EmbeddedWalletInsert{
			Token:        walletToken,
			WasmHash:     testWasmHash,
			WalletStatus: data.ProcessingWalletStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      walletToken,
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		assert.ErrorContains(t, err, "not found or wrong type")
	})

	t.Run("returns error when distribution account is missing", func(t *testing.T) {
		walletToken := uuid.NewString()
		_, err := testCtx.sdpModel.EmbeddedWallets.Insert(ctx, dbConnectionPool, data.EmbeddedWalletInsert{
			Token:        walletToken,
			WasmHash:     testWasmHash,
			WalletStatus: data.ProcessingWalletStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      walletToken,
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		// Update to SUCCESS without distribution account
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = $1, status=$2 WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, fmt.Sprintf("wallet_hash_%s", tssTransaction.ID), txSubStore.TransactionStatusProcessing, tssTransaction.ID)
		require.NoError(t, err)

		tssTransaction, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tssTransaction)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		assert.ErrorContains(t, err, "expected successful transaction")
		assert.ErrorContains(t, err, "to have a distribution account")
	})

	t.Run("returns error when SUCCESS transaction is missing stellar transaction hash", func(t *testing.T) {
		walletToken := uuid.NewString()
		_, err := testCtx.sdpModel.EmbeddedWallets.Insert(ctx, dbConnectionPool, data.EmbeddedWalletInsert{
			Token:        walletToken,
			WasmHash:     testWasmHash,
			WalletStatus: data.ProcessingWalletStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      walletToken,
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		// Update to SUCCESS without stellar transaction hash (this should fail validation)
		q := `UPDATE submitter_transactions SET status=$1, distribution_account=$2, completed_at=NOW() WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, txSubStore.TransactionStatusSuccess, testDistributionAccount, tssTransaction.ID)
		require.NoError(t, err)

		// Verify the transaction has no stellar transaction hash
		assert.False(t, tssTransaction.StellarTransactionHash.Valid, "test setup: transaction should not have stellar transaction hash")

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		assert.ErrorContains(t, err, "expected successful transaction")
		assert.ErrorContains(t, err, "to have a stellar transaction hash")
	})
}

func Test_WalletCreationFromSubmitterService_SyncBatchTransactions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupEmbeddedWalletTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	service := NewWalletCreationFromSubmitterService(testCtx.sdpModel, dbConnectionPool, testNetworkPassphrase)

	// Create test embedded wallets
	wallet1Token := uuid.NewString()
	wallet2Token := uuid.NewString()
	wallet3Token := uuid.NewString()
	wallet4Token := uuid.NewString()
	wallet5Token := uuid.NewString()

	walletTokens := []string{wallet1Token, wallet2Token, wallet3Token, wallet4Token, wallet5Token}

	// Create embedded wallet fixtures
	for _, token := range walletTokens {
		_, err := testCtx.sdpModel.EmbeddedWallets.Insert(ctx, dbConnectionPool, data.EmbeddedWalletInsert{
			Token:        token,
			WasmHash:     testWasmHash,
			WalletStatus: data.ProcessingWalletStatus,
		})
		require.NoError(t, err)
	}

	// Create TSS wallet creation transactions
	transactions := createEmbeddedWalletTSSTxs(t, testCtx, walletTokens...)

	// Update Hash and status of transactions to simulate success
	prepareEmbeddedWalletTxsForSync(t, testCtx, transactions)

	// Fail the last two transactions
	updatedTransactions := updateEmbeddedWalletTSSTransactionsToError(t, testCtx, []embeddedWalletPayloadToUpdateTSSTxToError{
		{transactionID: transactions[3].ID, statusMessages: "test-wallet-error"},
		{transactionID: transactions[4].ID, statusMessages: "another-wallet-test-error"},
	})
	require.Len(t, updatedTransactions, 2)
	for _, updatedTransaction := range updatedTransactions {
		utx := updatedTransaction
		for i, transaction := range transactions {
			if updatedTransaction.ID == transaction.ID {
				transactions[i] = &utx
				break
			}
		}
	}

	t.Run("sync embedded wallet transactions successfully", func(t *testing.T) {
		// We call sync batch transactions for all txs
		err := service.SyncBatchTransactions(ctx, len(transactions), testCtx.tenantID)
		require.NoError(t, err)

		// Check that successful wallet creations are updated
		for _, token := range walletTokens[:3] { // First 3 should succeed
			wallet, walletErr := testCtx.sdpModel.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, token)
			require.NoError(t, walletErr)
			require.Equal(t, data.SuccessWalletStatus, wallet.WalletStatus)
			require.NotEmpty(t, wallet.ContractAddress)

			// Verify the contract address is valid
			assert.True(t, strkey.IsValidContractAddress(wallet.ContractAddress), "contract address should be a valid stellar contract address")

			txs, txErr := testCtx.tssModel.GetAllByExternalIDs(ctx, []string{token})
			require.NoError(t, txErr)
			require.Len(t, txs, 1)
			require.Equal(t, fmt.Sprintf("test-hash-%s", txs[0].ID), txs[0].StellarTransactionHash.String)
		}

		// Check that failed wallet creations are updated
		for _, token := range walletTokens[3:] { // Last 2 should fail
			wallet, walletErr := testCtx.sdpModel.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, token)
			require.NoError(t, walletErr)
			require.Equal(t, data.FailedWalletStatus, wallet.WalletStatus)
			require.Empty(t, wallet.ContractAddress)
		}

		// Validate transactions synced_at is updated
		txs, txErr := testCtx.tssModel.GetAllByExternalIDs(ctx, walletTokens)
		require.NoError(t, txErr)
		require.Len(t, txs, 5)

		for _, tx := range txs {
			require.NotNil(t, tx.SyncedAt)
		}
	})

	t.Run("error when distribution account is missing", func(t *testing.T) {
		prepareEmbeddedWalletTxsForSync(t, testCtx, transactions)
		q := `UPDATE submitter_transactions SET distribution_account = NULL WHERE id = $1`
		_, err := dbConnectionPool.ExecContext(ctx, q, transactions[0].ID)
		require.NoError(t, err)

		err = service.SyncBatchTransactions(ctx, len(transactions), testCtx.tenantID)
		require.Error(t, err)
		require.ErrorContains(t, err, "expected successful transaction")
		require.ErrorContains(t, err, "to have a distribution account")
	})

	t.Run("error for orphaned embedded wallet transactions", func(t *testing.T) {
		prepareEmbeddedWalletTxsForSync(t, testCtx, transactions)
		// Insert a transaction that is not associated with an embedded wallet
		orphanToken := "orphan_wallet_token"

		tenantID := uuid.NewString()
		tx, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      orphanToken,
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: tenantID,
		})
		require.NoError(t, err)

		// Update transactions states PENDING->PROCESSING:
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'orphan_hash_123', status=$1, distribution_account=$2 WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tx, q, txSubStore.TransactionStatusProcessing, testDistributionAccount, tx.ID)
		require.NoError(t, err)

		tx, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tx)
		require.NoError(t, err)
		assert.Equal(t, txSubStore.TransactionStatusSuccess, tx.Status)
		assert.NotEmpty(t, tx.CompletedAt)

		err = service.SyncBatchTransactions(ctx, len(transactions)+1, tenantID)
		assert.ErrorContains(t, err, fmt.Sprintf("embedded wallet with token %s not found", orphanToken))
	})

	t.Run("filters payment transactions correctly", func(t *testing.T) {
		prepareEmbeddedWalletTxsForSync(t, testCtx, transactions)

		// Insert a payment transaction that should be ignored
		paymentTx, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      "payment-external-id",
			TransactionType: txSubStore.TransactionTypePayment,
			Payment: txSubStore.Payment{
				AssetCode:   "USDC",
				AssetIssuer: "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5",
				Amount:      100,
				Destination: "GCLWGQPMKXQSPF776IU33AH4PZNOOWNAWGGKVTBQMIC5IMKUNP3E6NVU",
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		// Update payment transaction to SUCCESS
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'payment_hash_123', status=$1, distribution_account=$2 WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, paymentTx, q, txSubStore.TransactionStatusProcessing, testDistributionAccount, paymentTx.ID)
		require.NoError(t, err)

		paymentTx, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *paymentTx)
		require.NoError(t, err)

		// Sync should succeed and skip the payment transaction
		err = service.SyncBatchTransactions(ctx, len(transactions)+1, testCtx.tenantID)
		require.NoError(t, err)

		// Verify payment transaction was NOT marked as synced (since it was skipped)
		pendingPaymentTx, err := testCtx.tssModel.GetTransactionPendingUpdateByID(ctx, dbConnectionPool, paymentTx.ID, txSubStore.TransactionTypePayment)
		require.NoError(t, err)
		assert.Equal(t, paymentTx.ID, pendingPaymentTx.ID, "payment transaction should still be pending sync since embedded wallet service ignores it")
	})

	t.Run("handles empty batch gracefully", func(t *testing.T) {
		differentTenantID := uuid.NewString()
		err := service.SyncBatchTransactions(ctx, 10, differentTenantID)
		require.NoError(t, err) // Should not error on empty batch
	})

	t.Run("sync embedded wallet transactions with RPC failures", func(t *testing.T) {
		// Create test embedded wallets for RPC failure scenario
		wallet1Token := uuid.NewString()
		wallet2Token := uuid.NewString()
		rpcFailWalletTokens := []string{wallet1Token, wallet2Token}

		// Create embedded wallet fixtures
		for _, token := range rpcFailWalletTokens {
			_, err := testCtx.sdpModel.EmbeddedWallets.Insert(ctx, dbConnectionPool, data.EmbeddedWalletInsert{
				Token:        token,
				WasmHash:     testWasmHash,
				WalletStatus: data.ProcessingWalletStatus,
			})
			require.NoError(t, err)
		}

		// Create TSS wallet creation transactions
		rpcFailTransactions := createEmbeddedWalletTSSTxs(t, testCtx, rpcFailWalletTokens...)

		// Set distribution account for all transactions
		for _, tx := range rpcFailTransactions {
			q := `UPDATE submitter_transactions SET distribution_account=$1 WHERE id = $2`
			_, err := testCtx.tssModel.DBConnectionPool.ExecContext(testCtx.ctx, q, testDistributionAccount, tx.ID)
			require.NoError(t, err)
		}

		// Simulate RPC simulation failures by manually setting ERROR status without stellar transaction hash
		for i, tx := range rpcFailTransactions {
			errorMsg := fmt.Sprintf("RPC simulation failed: test error %d", i+1)
			q := `UPDATE submitter_transactions SET status=$1, status_message=$2, completed_at=NOW(), stellar_transaction_hash=NULL WHERE id = $3`
			_, err := testCtx.tssModel.DBConnectionPool.ExecContext(testCtx.ctx, q, txSubStore.TransactionStatusError, errorMsg, tx.ID)
			require.NoError(t, err)
		}

		err := service.SyncBatchTransactions(ctx, len(rpcFailTransactions), testCtx.tenantID)
		require.NoError(t, err)

		// Check that RPC failed wallet creations are properly synced and marked as failed
		for _, token := range rpcFailWalletTokens {
			wallet, walletErr := testCtx.sdpModel.EmbeddedWallets.GetByToken(ctx, dbConnectionPool, token)
			require.NoError(t, walletErr)
			require.Equal(t, data.FailedWalletStatus, wallet.WalletStatus)
			require.Empty(t, wallet.ContractAddress)
		}

		// Validate transactions synced_at is updated despite having no stellar transaction hash
		txs, txErr := testCtx.tssModel.GetAllByExternalIDs(ctx, rpcFailWalletTokens)
		require.NoError(t, txErr)
		require.Len(t, txs, 2)

		for _, tx := range txs {
			require.NotNil(t, tx.SyncedAt)
			require.False(t, tx.StellarTransactionHash.Valid)
			require.Equal(t, txSubStore.TransactionStatusError, tx.Status)
		}
	})
}

func Test_WalletCreationFromSubmitterService_calculateContractAddress(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupEmbeddedWalletTestContext(t, dbConnectionPool)
	networkPassphrase := "Test SDF Network ; September 2015"

	service := NewWalletCreationFromSubmitterService(testCtx.sdpModel, dbConnectionPool, networkPassphrase)

	t.Run("successfully calculates contract address", func(t *testing.T) {
		distributionAccount := "GCLWGQPMKXQSPF776IU33AH4PZNOOWNAWGGKVTBQMIC5IMKUNP3E6NVU"

		contractAddress, err := service.calculateContractAddress(distributionAccount, testPublicKey)
		require.NoError(t, err)
		assert.NotEmpty(t, contractAddress)
		assert.True(t, strkey.IsValidContractAddress(contractAddress), "contract address should be a valid stellar contract address")
	})

	t.Run("returns error for invalid public key", func(t *testing.T) {
		distributionAccount := "GCLWGQPMKXQSPF776IU33AH4PZNOOWNAWGGKVTBQMIC5IMKUNP3E6NVU"
		invalidPublicKey := "invalid-hex"

		_, err := service.calculateContractAddress(distributionAccount, invalidPublicKey)
		assert.ErrorContains(t, err, "decoding public key")
	})

	t.Run("panics for invalid distribution account", func(t *testing.T) {
		invalidDistributionAccount := "invalid-account"

		_, err := service.calculateContractAddress(invalidDistributionAccount, testPublicKey)
		assert.ErrorContains(t, err, "decoding distribution account address")
	})
}

// Helper functions for embedded wallet tests

func createEmbeddedWalletTSSTxs(t *testing.T, testCtx *testContext, walletTokens ...string) []*txSubStore.Transaction {
	t.Helper()

	walletTokensQuantity := len(walletTokens)
	transactionsToCreate := make([]txSubStore.Transaction, 0, walletTokensQuantity)
	for _, token := range walletTokens {
		transactionsToCreate = append(transactionsToCreate, txSubStore.Transaction{
			ExternalID:      token,
			TransactionType: txSubStore.TransactionTypeWalletCreation,
			WalletCreation: txSubStore.WalletCreation{
				PublicKey: testPublicKey,
				WasmHash:  testWasmHash,
			},
			TenantID: testCtx.tenantID,
		})
	}

	transactionsCreated, err := testCtx.tssModel.BulkInsert(testCtx.ctx, testCtx.tssModel.DBConnectionPool, transactionsToCreate)
	require.NoError(t, err)
	require.Len(t, transactionsCreated, walletTokensQuantity)

	transactions := make([]*txSubStore.Transaction, 0, walletTokensQuantity)
	for i := range transactionsCreated {
		transactions = append(transactions, &transactionsCreated[i])
	}

	return transactions
}

func prepareEmbeddedWalletTxsForSync(t *testing.T, testCtx *testContext, transactions []*txSubStore.Transaction) {
	t.Helper()

	txLen := len(transactions)

	var err error

	for _, tx := range transactions {
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = $1, status=$2, distribution_account=$3 WHERE id = $4`
		_, err = testCtx.tssModel.DBConnectionPool.ExecContext(testCtx.ctx, q, "test-hash-"+tx.ID, txSubStore.TransactionStatusProcessing, testDistributionAccount, tx.ID)
		require.NoError(t, err)

		tx, err = testCtx.tssModel.Get(testCtx.ctx, tx.ID)
		require.NoError(t, err)

		// Update transactions states PROCESSING->SUCCESS:
		if tx.Status == txSubStore.TransactionStatusProcessing {
			tx, err = testCtx.tssModel.UpdateStatusToSuccess(testCtx.ctx, *tx)
			require.NoError(t, err)
			assert.Equal(t, txSubStore.TransactionStatusSuccess, tx.Status)
			assert.NotEmpty(t, tx.CompletedAt)
		}
	}

	transactionIDs := make([]string, txLen)
	for i, tx := range transactions {
		transactionIDs[i] = tx.ID
	}

	unsyncEmbeddedWalletTransactions(t, testCtx, transactionIDs)
}

func unsyncEmbeddedWalletTransactions(t *testing.T, testCtx *testContext, transactionIDs []string) {
	t.Helper()

	query := `UPDATE submitter_transactions SET synced_at = NULL WHERE id = ANY($1)`
	_, err := testCtx.sdpModel.DBConnectionPool.ExecContext(testCtx.ctx, query, pq.Array(transactionIDs))
	require.NoError(t, err)
}

type embeddedWalletPayloadToUpdateTSSTxToError struct {
	transactionID  string
	statusMessages string
}

func updateEmbeddedWalletTSSTransactionsToError(t *testing.T, testCtx *testContext, txDataSlice []embeddedWalletPayloadToUpdateTSSTxToError) []txSubStore.Transaction {
	t.Helper()

	var updatedTransactions []txSubStore.Transaction

	for _, txData := range txDataSlice {
		tx, err := testCtx.tssModel.Get(testCtx.ctx, txData.transactionID)
		require.NoError(t, err)

		// If transaction is already SUCCESS, we need to reset it to PROCESSING first
		if tx.Status == txSubStore.TransactionStatusSuccess {
			q := `UPDATE submitter_transactions SET status = $1, completed_at = NULL WHERE id = $2 RETURNING ` + txSubStore.TransactionColumnNames("", "")
			err = testCtx.tssModel.DBConnectionPool.GetContext(testCtx.ctx, tx, q, txSubStore.TransactionStatusProcessing, tx.ID)
			require.NoError(t, err)
		}

		tx, err = testCtx.tssModel.UpdateStatusToError(testCtx.ctx, *tx, txData.statusMessages)
		require.NoError(t, err)
		require.Equal(t, txSubStore.TransactionStatusError, tx.Status)
		require.Equal(t, txData.statusMessages, tx.StatusMessage.String)

		updatedTransactions = append(updatedTransactions, *tx)
	}

	return updatedTransactions
}
