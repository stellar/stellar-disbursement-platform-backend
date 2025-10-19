package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

const (
	testSponsoredAccount = "CAMAMZUOULVWFAB3KRROW5ELPUFHSEKPUALORCFBLFX7XBWWUCUJLR53"
	testOperationXDR     = "AAAAAAAAAAHXkotywnA8z+r365/0701QSlWouXn8m0UOoshCtNHOYQAAAAh0cmFuc2ZlcgAAAAAAAAAA"
	testTransactionHash  = "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
)

func setupSponsoredTransactionTestContext(t *testing.T, dbConnectionPool db.DBConnectionPool) *testContext {
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

func Test_SponsoredTransactionFromSubmitterService_SyncTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupSponsoredTransactionTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	service := NewSponsoredTransactionFromSubmitterService(testCtx.sdpModel, dbConnectionPool)

	sponsoredTransactionID := uuid.NewString()
	_, err := testCtx.sdpModel.SponsoredTransactions.Insert(ctx, dbConnectionPool, data.SponsoredTransactionInsert{
		ID:           sponsoredTransactionID,
		Account:      testSponsoredAccount,
		OperationXDR: testOperationXDR,
		Status:       data.PendingSponsoredTransactionStatus,
	})
	require.NoError(t, err)

	t.Run("successfully syncs successful sponsored transaction", func(t *testing.T) {
		// Create successful sponsored transaction
		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      sponsoredTransactionID,
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		q := `UPDATE submitter_transactions SET stellar_transaction_hash = $1, status=$2, distribution_account=$3 WHERE id = $4 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, testTransactionHash, txSubStore.TransactionStatusProcessing, testDistributionAccount, tssTransaction.ID)
		require.NoError(t, err)

		tssTransaction, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tssTransaction)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		require.NoError(t, err)

		updatedSponsoredTx, err := testCtx.sdpModel.SponsoredTransactions.GetByID(ctx, dbConnectionPool, sponsoredTransactionID)
		require.NoError(t, err)
		assert.Equal(t, string(data.SuccessSponsoredTransactionStatus), updatedSponsoredTx.Status)
		assert.Equal(t, testTransactionHash, updatedSponsoredTx.TransactionHash)

		// Verify transaction was marked as synced
		_, err = testCtx.tssModel.GetTransactionPendingUpdateByID(ctx, dbConnectionPool, tssTransaction.ID, txSubStore.TransactionTypeSponsored)
		assert.ErrorIs(t, err, txSubStore.ErrRecordNotFound, "transaction should be marked as synced")
	})

	t.Run("successfully syncs failed sponsored transaction", func(t *testing.T) {
		// Create another sponsored transaction for failed test
		failedSponsoredTransactionID := uuid.NewString()
		_, err := testCtx.sdpModel.SponsoredTransactions.Insert(ctx, dbConnectionPool, data.SponsoredTransactionInsert{
			ID:           failedSponsoredTransactionID,
			Account:      testSponsoredAccount,
			OperationXDR: testOperationXDR,
			Status:       data.PendingSponsoredTransactionStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      failedSponsoredTransactionID,
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'error_hash_123', status=$1, status_message=$2 WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, txSubStore.TransactionStatusError, "Transaction failed", tssTransaction.ID)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		require.NoError(t, err)

		updatedSponsoredTx, err := testCtx.sdpModel.SponsoredTransactions.GetByID(ctx, dbConnectionPool, failedSponsoredTransactionID)
		require.NoError(t, err)
		assert.Equal(t, string(data.FailedSponsoredTransactionStatus), updatedSponsoredTx.Status)
		assert.Empty(t, updatedSponsoredTx.TransactionHash)

		_, err = testCtx.tssModel.GetTransactionPendingUpdateByID(ctx, dbConnectionPool, tssTransaction.ID, txSubStore.TransactionTypeSponsored)
		assert.ErrorIs(t, err, txSubStore.ErrRecordNotFound, "transaction should be marked as synced")
	})

	t.Run("successfully syncs failed sponsored transaction (RPC simulation failure)", func(t *testing.T) {
		// Create sponsored transaction for RPC simulation failure test
		rpcFailSponsoredTransactionID := uuid.NewString()
		_, err := testCtx.sdpModel.SponsoredTransactions.Insert(ctx, dbConnectionPool, data.SponsoredTransactionInsert{
			ID:           rpcFailSponsoredTransactionID,
			Account:      testSponsoredAccount,
			OperationXDR: testOperationXDR,
			Status:       data.ProcessingSponsoredTransactionStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      rpcFailSponsoredTransactionID,
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		// Simulate RPC simulation failure by updating to ERROR without stellar transaction hash or distribution account
		q := `UPDATE submitter_transactions SET status=$1, status_message=$2, completed_at=NOW() WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, txSubStore.TransactionStatusError, "RPC simulation failed: invalid operation", tssTransaction.ID)
		require.NoError(t, err)

		assert.False(t, tssTransaction.StellarTransactionHash.Valid)
		assert.False(t, tssTransaction.DistributionAccount.Valid)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		require.NoError(t, err)

		updatedSponsoredTx, err := testCtx.sdpModel.SponsoredTransactions.GetByID(ctx, dbConnectionPool, rpcFailSponsoredTransactionID)
		require.NoError(t, err)
		assert.Equal(t, string(data.FailedSponsoredTransactionStatus), updatedSponsoredTx.Status)
		assert.Empty(t, updatedSponsoredTx.TransactionHash)

		_, err = testCtx.tssModel.GetTransactionPendingUpdateByID(ctx, dbConnectionPool, tssTransaction.ID, txSubStore.TransactionTypeSponsored)
		assert.ErrorIs(t, err, txSubStore.ErrRecordNotFound, "transaction should be marked as synced")
	})
}

func Test_SponsoredTransactionFromSubmitterService_SyncTransaction_errors(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupSponsoredTransactionTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	service := NewSponsoredTransactionFromSubmitterService(testCtx.sdpModel, dbConnectionPool)

	t.Run("returns error for non-existent transaction", func(t *testing.T) {
		err := service.SyncTransaction(ctx, "non-existent-tx-id")
		assert.ErrorContains(t, err, "sponsored transaction non-existent-tx-id not found or wrong type")
	})

	t.Run("returns error for non-sponsored transaction", func(t *testing.T) {
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
		assert.ErrorContains(t, err, "sponsored transaction")
		assert.ErrorContains(t, err, "not found or wrong type")
	})

	t.Run("returns error for non-existent sponsored transaction", func(t *testing.T) {
		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      "non-existent-sponsored-tx",
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		// Update with both stellar_transaction_hash and distribution_account so validation passes
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = $1, status=$2, distribution_account=$3 WHERE id = $4 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, testTransactionHash, txSubStore.TransactionStatusProcessing, testDistributionAccount, tssTransaction.ID)
		require.NoError(t, err)

		tssTransaction, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tssTransaction)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		assert.ErrorContains(t, err, "sponsored transaction with ID non-existent-sponsored-tx not found")
	})

	t.Run("returns error when transaction is not in terminal state", func(t *testing.T) {
		sponsoredTransactionID := uuid.NewString()
		_, err := testCtx.sdpModel.SponsoredTransactions.Insert(ctx, dbConnectionPool, data.SponsoredTransactionInsert{
			ID:           sponsoredTransactionID,
			Account:      testSponsoredAccount,
			OperationXDR: testOperationXDR,
			Status:       data.ProcessingSponsoredTransactionStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      sponsoredTransactionID,
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		assert.ErrorContains(t, err, "not found or wrong type")
	})

	t.Run("returns error when distribution account is missing", func(t *testing.T) {
		sponsoredTransactionID := uuid.NewString()
		_, err := testCtx.sdpModel.SponsoredTransactions.Insert(ctx, dbConnectionPool, data.SponsoredTransactionInsert{
			ID:           sponsoredTransactionID,
			Account:      testSponsoredAccount,
			OperationXDR: testOperationXDR,
			Status:       data.PendingSponsoredTransactionStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      sponsoredTransactionID,
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		// Update to SUCCESS without distribution account
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = $1, status=$2 WHERE id = $3 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, fmt.Sprintf("sponsored_hash_%s", tssTransaction.ID), txSubStore.TransactionStatusProcessing, tssTransaction.ID)
		require.NoError(t, err)

		tssTransaction, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tssTransaction)
		require.NoError(t, err)

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		assert.ErrorContains(t, err, "expected successful transaction")
		assert.ErrorContains(t, err, "to have a distribution account")
	})

	t.Run("returns error when SUCCESS transaction is missing stellar transaction hash", func(t *testing.T) {
		sponsoredTransactionID := uuid.NewString()
		_, err := testCtx.sdpModel.SponsoredTransactions.Insert(ctx, dbConnectionPool, data.SponsoredTransactionInsert{
			ID:           sponsoredTransactionID,
			Account:      testSponsoredAccount,
			OperationXDR: testOperationXDR,
			Status:       data.ProcessingSponsoredTransactionStatus,
		})
		require.NoError(t, err)

		tssTransaction, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      sponsoredTransactionID,
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: testCtx.tenantID,
		})
		require.NoError(t, err)

		// Update to SUCCESS without stellar transaction hash (this should fail validation)
		q := `UPDATE submitter_transactions SET status=$1, completed_at=NOW() WHERE id = $2 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tssTransaction, q, txSubStore.TransactionStatusSuccess, tssTransaction.ID)
		require.NoError(t, err)

		// Verify the transaction has no stellar transaction hash
		assert.False(t, tssTransaction.StellarTransactionHash.Valid, "test setup: transaction should not have stellar transaction hash")

		err = service.SyncTransaction(ctx, tssTransaction.ID)
		assert.ErrorContains(t, err, "expected successful transaction")
		assert.ErrorContains(t, err, "to have a stellar transaction hash")
	})
}

func Test_SponsoredTransactionFromSubmitterService_SyncBatchTransactions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupSponsoredTransactionTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	service := NewSponsoredTransactionFromSubmitterService(testCtx.sdpModel, dbConnectionPool)

	// Create test sponsored transactions
	sponsoredTx1ID := uuid.NewString()
	sponsoredTx2ID := uuid.NewString()
	sponsoredTx3ID := uuid.NewString()
	sponsoredTx4ID := uuid.NewString()
	sponsoredTx5ID := uuid.NewString()

	sponsoredTxIDs := []string{sponsoredTx1ID, sponsoredTx2ID, sponsoredTx3ID, sponsoredTx4ID, sponsoredTx5ID}

	// Create sponsored transaction fixtures
	for _, id := range sponsoredTxIDs {
		_, err := testCtx.sdpModel.SponsoredTransactions.Insert(ctx, dbConnectionPool, data.SponsoredTransactionInsert{
			ID:           id,
			Account:      testSponsoredAccount,
			OperationXDR: testOperationXDR,
			Status:       data.ProcessingSponsoredTransactionStatus,
		})
		require.NoError(t, err)
	}

	// Create TSS sponsored transactions
	transactions := createSponsoredTSSTxs(t, testCtx, sponsoredTxIDs...)

	// Update status of transactions to simulate success
	prepareSponsoredTxsForSync(t, testCtx, transactions)

	// Fail the last two transactions
	updatedTransactions := updateSponsoredTSSTransactionsToError(t, testCtx, []sponsoredPayloadToUpdateTSSTxToError{
		{transactionID: transactions[3].ID, statusMessages: "test-sponsored-error"},
		{transactionID: transactions[4].ID, statusMessages: "another-sponsored-test-error"},
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

	t.Run("sync sponsored transactions successfully", func(t *testing.T) {
		// We call sync batch transactions for all txs
		err := service.SyncBatchTransactions(ctx, len(transactions), testCtx.tenantID)
		require.NoError(t, err)

		// Check that successful sponsored transactions are updated
		for _, id := range sponsoredTxIDs[:3] { // First 3 should succeed
			sponsoredTx, sponsoredErr := testCtx.sdpModel.SponsoredTransactions.GetByID(ctx, dbConnectionPool, id)
			require.NoError(t, sponsoredErr)
			require.Equal(t, string(data.SuccessSponsoredTransactionStatus), sponsoredTx.Status)
			require.NotEmpty(t, sponsoredTx.TransactionHash)

			txs, txErr := testCtx.tssModel.GetAllByExternalIDs(ctx, []string{id})
			require.NoError(t, txErr)
			require.Len(t, txs, 1)
			require.Len(t, txs[0].StellarTransactionHash.String, 64)
		}

		// Check that failed sponsored transactions are updated
		for _, id := range sponsoredTxIDs[3:] { // Last 2 should fail
			sponsoredTx, sponsoredErr := testCtx.sdpModel.SponsoredTransactions.GetByID(ctx, dbConnectionPool, id)
			require.NoError(t, sponsoredErr)
			require.Equal(t, string(data.FailedSponsoredTransactionStatus), sponsoredTx.Status)
			require.Empty(t, sponsoredTx.TransactionHash)
		}

		// Validate transactions synced_at is updated
		txs, txErr := testCtx.tssModel.GetAllByExternalIDs(ctx, sponsoredTxIDs)
		require.NoError(t, txErr)
		require.Len(t, txs, 5)

		for _, tx := range txs {
			require.NotNil(t, tx.SyncedAt)
		}
	})

	t.Run("error when distribution account is missing", func(t *testing.T) {
		prepareSponsoredTxsForSync(t, testCtx, transactions)
		q := `UPDATE submitter_transactions SET distribution_account = NULL WHERE id = $1`
		_, err := dbConnectionPool.ExecContext(ctx, q, transactions[0].ID)
		require.NoError(t, err)

		err = service.SyncBatchTransactions(ctx, len(transactions), testCtx.tenantID)
		require.Error(t, err)
		require.ErrorContains(t, err, "expected successful transaction")
		require.ErrorContains(t, err, "to have a distribution account")
	})

	t.Run("error for orphaned sponsored transactions", func(t *testing.T) {
		prepareSponsoredTxsForSync(t, testCtx, transactions)
		// Insert a transaction that is not associated with a sponsored transaction
		orphanID := "orphan_sponsored_tx_id"

		tenantID := uuid.NewString()
		tx, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:      orphanID,
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: tenantID,
		})
		require.NoError(t, err)

		// Update transactions states PENDING->PROCESSING with both stellar_transaction_hash and distribution_account:
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = $1, status=$2, distribution_account=$3 WHERE id = $4 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tx, q, testTransactionHash, txSubStore.TransactionStatusProcessing, testSponsoredAccount, tx.ID)
		require.NoError(t, err)

		tx, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tx)
		require.NoError(t, err)
		assert.Equal(t, txSubStore.TransactionStatusSuccess, tx.Status)
		assert.NotEmpty(t, tx.CompletedAt)

		err = service.SyncBatchTransactions(ctx, len(transactions)+1, tenantID)
		assert.ErrorContains(t, err, fmt.Sprintf("sponsored transaction with ID %s not found", orphanID))
	})

	t.Run("filters payment transactions correctly", func(t *testing.T) {
		prepareSponsoredTxsForSync(t, testCtx, transactions)

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
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'payment_hash_123', status=$1 WHERE id = $2 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, paymentTx, q, txSubStore.TransactionStatusProcessing, paymentTx.ID)
		require.NoError(t, err)

		paymentTx, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *paymentTx)
		require.NoError(t, err)

		// Sync should succeed and skip the payment transaction
		err = service.SyncBatchTransactions(ctx, len(transactions)+1, testCtx.tenantID)
		require.NoError(t, err)

		// Verify payment transaction was NOT marked as synced (since it was skipped)
		pendingPaymentTx, err := testCtx.tssModel.GetTransactionPendingUpdateByID(ctx, dbConnectionPool, paymentTx.ID, txSubStore.TransactionTypePayment)
		require.NoError(t, err)
		assert.Equal(t, paymentTx.ID, pendingPaymentTx.ID, "payment transaction should still be pending sync since sponsored service ignores it")
	})

	t.Run("handles empty batch gracefully", func(t *testing.T) {
		differentTenantID := uuid.NewString()
		err := service.SyncBatchTransactions(ctx, 10, differentTenantID)
		require.NoError(t, err) // Should not error on empty batch
	})
}

// Helper functions for sponsored transaction tests

func createSponsoredTSSTxs(t *testing.T, testCtx *testContext, sponsoredTxIDs ...string) []*txSubStore.Transaction {
	t.Helper()

	sponsoredTxsQuantity := len(sponsoredTxIDs)
	transactionsToCreate := make([]txSubStore.Transaction, 0, sponsoredTxsQuantity)
	for _, id := range sponsoredTxIDs {
		transactionsToCreate = append(transactionsToCreate, txSubStore.Transaction{
			ExternalID:      id,
			TransactionType: txSubStore.TransactionTypeSponsored,
			Sponsored: txSubStore.Sponsored{
				SponsoredAccount:      testSponsoredAccount,
				SponsoredOperationXDR: testOperationXDR,
			},
			TenantID: testCtx.tenantID,
		})
	}

	transactionsCreated, err := testCtx.tssModel.BulkInsert(testCtx.ctx, testCtx.tssModel.DBConnectionPool, transactionsToCreate)
	require.NoError(t, err)
	require.Len(t, transactionsCreated, sponsoredTxsQuantity)

	transactions := make([]*txSubStore.Transaction, 0, sponsoredTxsQuantity)
	for i := range transactionsCreated {
		transactions = append(transactions, &transactionsCreated[i])
	}

	return transactions
}

func prepareSponsoredTxsForSync(t *testing.T, testCtx *testContext, transactions []*txSubStore.Transaction) {
	t.Helper()

	txLen := len(transactions)

	var err error

	for _, tx := range transactions {
		hashBytes := make([]byte, 32)
		_, err = rand.Read(hashBytes)
		require.NoError(t, err)
		txHash := hex.EncodeToString(hashBytes)

		q := `UPDATE submitter_transactions SET stellar_transaction_hash = $1, status=$2, distribution_account=$3 WHERE id = $4`
		_, err = testCtx.tssModel.DBConnectionPool.ExecContext(testCtx.ctx, q, txHash, txSubStore.TransactionStatusProcessing, testSponsoredAccount, tx.ID)
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

	unsyncSponsoredTransactions(t, testCtx, transactionIDs)
}

func unsyncSponsoredTransactions(t *testing.T, testCtx *testContext, transactionIDs []string) {
	t.Helper()

	query := `UPDATE submitter_transactions SET synced_at = NULL WHERE id = ANY($1)`
	_, err := testCtx.sdpModel.DBConnectionPool.ExecContext(testCtx.ctx, query, pq.Array(transactionIDs))
	require.NoError(t, err)
}

type sponsoredPayloadToUpdateTSSTxToError struct {
	transactionID  string
	statusMessages string
}

func updateSponsoredTSSTransactionsToError(t *testing.T, testCtx *testContext, txDataSlice []sponsoredPayloadToUpdateTSSTxToError) []txSubStore.Transaction {
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
