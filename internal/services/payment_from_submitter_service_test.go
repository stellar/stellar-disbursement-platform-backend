package services

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events/schemas"
	txSubStore "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

type testContext struct {
	tssModel *txSubStore.TransactionModel
	sdpModel *data.Models
	ctx      context.Context
	tenantID string
}

func setupTestContext(t *testing.T, dbConnectionPool db.DBConnectionPool) *testContext {
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

func Test_PaymentFromSubmitterService_SyncBatchTransactions(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	monitorService := NewPaymentFromSubmitterService(testCtx.sdpModel, dbConnectionPool)

	// create fixtures
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
		"My Wallet",
		"https://www.wallet.com",
		"www.wallet.com",
		"wallet1://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool,
		"USDC",
		"GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE")

	// create disbursements
	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Disbursements, &data.Disbursement{
		Name:   "ready disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	// create disbursement receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver3 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver4 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	rw1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw3 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw4 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver4.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw1,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})
	payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw2,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.ReadyPaymentStatus,
	})
	payment3 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw3,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "300",
		Status:         data.ReadyPaymentStatus,
	})
	payment4 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw4,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "400",
		Status:         data.ReadyPaymentStatus,
	})
	payment5 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw4,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "400",
		Status:         data.ReadyPaymentStatus,
	})

	payments := []*data.Payment{payment1, payment2, payment3, payment4, payment5}

	// Creating TSS transactions
	transactions := createTSSTxs(t, testCtx, payments...)

	// Update Hash and status of transactions to simulate success
	prepareTxsForSync(t, testCtx, transactions)

	// Fail the last transaction
	updatedTransactions := updateTSSTransactionsToError(t, testCtx, []payloadToUpdateTSSTxToError{
		{transactionID: transactions[3].ID, statusMessages: "test-error"},
		{transactionID: transactions[4].ID, statusMessages: "another-test-error"},
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

	t.Run("sync tss transactions successfully", func(t *testing.T) {
		// We call sync batch transactions for all txs
		err := monitorService.SyncBatchTransactions(ctx, len(transactions), testCtx.tenantID)
		require.NoError(t, err)

		// check that successful payments are updated
		for _, p := range []*data.Payment{payment1, payment2, payment3} {
			payment, paymentErr := testCtx.sdpModel.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, paymentErr)
			require.Equal(t, data.SuccessPaymentStatus, payment.Status)
			txs, txErr := testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{p.ID})
			require.NoError(t, txErr)
			require.Len(t, txs, 1)
			require.Equal(t, fmt.Sprintf("test-hash-%s", txs[0].ID), payment.StellarTransactionID)
		}

		// check that failed payment is updated
		payment, paymentErr := testCtx.sdpModel.Payment.Get(ctx, payment4.ID, dbConnectionPool)
		require.NoError(t, paymentErr)
		require.Equal(t, data.FailedPaymentStatus, payment.Status)
		txs, txErr := testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{payment4.ID})
		require.NoError(t, txErr)
		require.Len(t, txs, 1)
		require.Equal(t, fmt.Sprintf("test-hash-%s", txs[0].ID), payment.StellarTransactionID)
		require.Len(t, payment.StatusHistory, 3)
		require.Equal(t, payment.StatusHistory[2].Status, data.FailedPaymentStatus)
		require.Equal(t, payment.StatusHistory[2].StatusMessage, "test-error")

		payment, paymentErr = testCtx.sdpModel.Payment.Get(ctx, payment5.ID, dbConnectionPool)
		require.NoError(t, paymentErr)
		require.Equal(t, data.FailedPaymentStatus, payment.Status)
		txs, txErr = testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{payment5.ID})
		require.NoError(t, txErr)
		require.Len(t, txs, 1)
		require.Equal(t, fmt.Sprintf("test-hash-%s", txs[0].ID), payment.StellarTransactionID)
		require.Len(t, payment.StatusHistory, 3)
		require.Equal(t, payment.StatusHistory[2].Status, data.FailedPaymentStatus)
		require.Equal(t, payment.StatusHistory[2].StatusMessage, "another-test-error")

		// validate transactions synced_at is updated.
		txs, txErr = testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{payment1.ID, payment2.ID, payment3.ID, payment4.ID, payment5.ID})
		require.NoError(t, txErr)
		require.Len(t, txs, 5)

		for _, tx := range txs {
			require.NotNil(t, tx.SyncedAt)
		}
	})

	t.Run("error when hash is invalid", func(t *testing.T) {
		prepareTxsForSync(t, testCtx, transactions)
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = '' WHERE id = $1`
		_, err := dbConnectionPool.ExecContext(ctx, q, transactions[0].ID)
		require.NoError(t, err)

		err = monitorService.SyncBatchTransactions(ctx, len(transactions), testCtx.tenantID)
		require.Error(t, err)
		require.ErrorContainsf(t, err, "stellar transaction id is required", "error: %s", err.Error())
	})

	t.Run("payment is not pending", func(t *testing.T) {
		prepareTxsForSync(t, testCtx, transactions)
		updatePaymentStatus(t, testCtx, payment1.ID, data.SuccessPaymentStatus)

		err := monitorService.SyncBatchTransactions(ctx, len(transactions), testCtx.tenantID)
		require.Error(t, err)
		contains := fmt.Sprintf("updating payment ID %s for transaction ID %s: cannot transition from SUCCESS to SUCCESS for payment %s: cannot transition from SUCCESS to SUCCESS", payment1.ID, transactions[0].ID, payment1.ID)
		require.ErrorContainsf(t, err, contains, "error: %s", err.Error())
	})

	t.Run("error for orphaned transactions", func(t *testing.T) {
		prepareTxsForSync(t, testCtx, transactions)
		// insert a transaction that is not associated with a payment
		paymentID := "dummy_payment_id"

		tenantID := uuid.NewString()
		tx, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:  paymentID,
			AssetCode:   asset.Code,
			AssetIssuer: asset.Issuer,
			Amount:      100,
			Destination: rw1.StellarAddress,
			TenantID:    tenantID,
		})
		require.NoError(t, err)

		// Update transactions states PENDING->PROCESSING:
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'dummy_hash_123', status=$1 WHERE id = $2 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tx, q, txSubStore.TransactionStatusProcessing, tx.ID)
		require.NoError(t, err)

		tx, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tx)
		require.NoError(t, err)
		assert.Equal(t, txSubStore.TransactionStatusSuccess, tx.Status)
		assert.NotEmpty(t, tx.CompletedAt)

		err = monitorService.SyncBatchTransactions(ctx, len(transactions)+1, tenantID)
		assert.ErrorContains(t, err, fmt.Sprintf("expected exactly 1 payment for the transaction ID %s but found 0", tx.ID))
	})
}

func Test_PaymentFromSubmitterService_SyncTransaction(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	monitorService := NewPaymentFromSubmitterService(testCtx.sdpModel, dbConnectionPool)

	// create fixtures
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool,
		"My Wallet",
		"https://www.wallet.com",
		"www.wallet.com",
		"wallet1://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool,
		"USDC",
		"GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE")

	// create disbursements
	startedDisbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Disbursements, &data.Disbursement{
		Name:   "ready disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	// create disbursement receivers
	receiver1 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver2 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver3 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiver4 := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})

	rw1 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver1.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw2 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver2.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw3 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver3.ID, wallet.ID, data.RegisteredReceiversWalletStatus)
	rw4 := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver4.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	payment1 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw1,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "100",
		Status:         data.ReadyPaymentStatus,
	})
	payment2 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw2,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "200",
		Status:         data.ReadyPaymentStatus,
	})
	payment3 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw3,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "300",
		Status:         data.ReadyPaymentStatus,
	})
	payment4 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw4,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "400",
		Status:         data.ReadyPaymentStatus,
	})
	payment5 := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		ReceiverWallet: rw4,
		Disbursement:   startedDisbursement,
		Asset:          *asset,
		Amount:         "400",
		Status:         data.ReadyPaymentStatus,
	})

	payments := []*data.Payment{payment1, payment2, payment3, payment4, payment5}

	// Creating TSS transactions
	transactions := createTSSTxs(t, testCtx, payments...)

	// Update Hash and status of transactions to simulate success
	prepareTxsForSync(t, testCtx, transactions)

	// Fail the last transaction
	updatedTransactions := updateTSSTransactionsToError(t, testCtx, []payloadToUpdateTSSTxToError{
		{transactionID: transactions[3].ID, statusMessages: "test-error"},
		{transactionID: transactions[4].ID, statusMessages: "another-test-error"},
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

	t.Run("sync tss transactions successfully", func(t *testing.T) {
		// We call sync transaction for each tss transaction created
		for _, transaction := range transactions {
			err := monitorService.SyncTransaction(ctx, &schemas.EventPaymentCompletedData{TransactionID: transaction.ID})
			require.NoError(t, err)
		}

		// check that successful payments are updated
		for _, p := range []*data.Payment{payment1, payment2, payment3} {
			payment, paymentErr := testCtx.sdpModel.Payment.Get(ctx, p.ID, dbConnectionPool)
			require.NoError(t, paymentErr)
			require.Equal(t, data.SuccessPaymentStatus, payment.Status)
			txs, txErr := testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{p.ID})
			require.NoError(t, txErr)
			require.Len(t, txs, 1)
			require.Equal(t, fmt.Sprintf("test-hash-%s", txs[0].ID), payment.StellarTransactionID)
		}

		// check that failed payment is updated
		payment, paymentErr := testCtx.sdpModel.Payment.Get(ctx, payment4.ID, dbConnectionPool)
		require.NoError(t, paymentErr)
		require.Equal(t, data.FailedPaymentStatus, payment.Status)
		txs, txErr := testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{payment4.ID})
		require.NoError(t, txErr)
		require.Len(t, txs, 1)
		require.Equal(t, fmt.Sprintf("test-hash-%s", txs[0].ID), payment.StellarTransactionID)
		require.Len(t, payment.StatusHistory, 3)
		require.Equal(t, payment.StatusHistory[2].Status, data.FailedPaymentStatus)
		require.Equal(t, payment.StatusHistory[2].StatusMessage, "test-error")

		payment, paymentErr = testCtx.sdpModel.Payment.Get(ctx, payment5.ID, dbConnectionPool)
		require.NoError(t, paymentErr)
		require.Equal(t, data.FailedPaymentStatus, payment.Status)
		txs, txErr = testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{payment5.ID})
		require.NoError(t, txErr)
		require.Len(t, txs, 1)
		require.Equal(t, fmt.Sprintf("test-hash-%s", txs[0].ID), payment.StellarTransactionID)
		require.Len(t, payment.StatusHistory, 3)
		require.Equal(t, payment.StatusHistory[2].Status, data.FailedPaymentStatus)
		require.Equal(t, payment.StatusHistory[2].StatusMessage, "another-test-error")

		// validate transactions synced_at is updated.
		txs, txErr = testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{payment1.ID, payment2.ID, payment3.ID, payment4.ID, payment5.ID})
		require.NoError(t, txErr)
		require.Len(t, txs, 5)

		for _, tx := range txs {
			require.NotNil(t, tx.SyncedAt)
		}
	})

	t.Run("error when hash is invalid", func(t *testing.T) {
		prepareTxsForSync(t, testCtx, transactions)
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = '' WHERE id = $1`
		_, err := dbConnectionPool.ExecContext(ctx, q, transactions[0].ID)
		require.NoError(t, err)

		err = monitorService.SyncTransaction(ctx, &schemas.EventPaymentCompletedData{TransactionID: transactions[0].ID})
		require.Error(t, err)
		require.ErrorContainsf(t, err, "stellar transaction id is required", "error: %s", err.Error())
	})

	t.Run("payment is not pending", func(t *testing.T) {
		prepareTxsForSync(t, testCtx, transactions)
		updatePaymentStatus(t, testCtx, payment1.ID, data.SuccessPaymentStatus)

		err := monitorService.SyncTransaction(ctx, &schemas.EventPaymentCompletedData{TransactionID: transactions[0].ID})
		require.Error(t, err)
		contains := fmt.Sprintf("updating payment ID %s for transaction ID %s: cannot transition from SUCCESS to SUCCESS for payment %s: cannot transition from SUCCESS to SUCCESS", payment1.ID, transactions[0].ID, payment1.ID)
		require.ErrorContainsf(t, err, contains, "error: %s", err.Error())
	})

	t.Run("error for orphaned transactions", func(t *testing.T) {
		prepareTxsForSync(t, testCtx, transactions)
		// insert a transaction that is not associated with a payment
		paymentID := "dummy_payment_id"

		tx, err := testCtx.tssModel.Insert(ctx, txSubStore.Transaction{
			ExternalID:  paymentID,
			AssetCode:   asset.Code,
			AssetIssuer: asset.Issuer,
			Amount:      100,
			Destination: rw1.StellarAddress,
			TenantID:    uuid.NewString(),
		})
		require.NoError(t, err)

		// Update transactions states PENDING->PROCESSING:
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = 'dummy_hash_123', status=$1 WHERE id = $2 RETURNING ` + txSubStore.TransactionColumnNames("", "")
		err = dbConnectionPool.GetContext(ctx, tx, q, txSubStore.TransactionStatusProcessing, tx.ID)
		require.NoError(t, err)

		tx, err = testCtx.tssModel.UpdateStatusToSuccess(ctx, *tx)
		require.NoError(t, err)
		assert.Equal(t, txSubStore.TransactionStatusSuccess, tx.Status)
		assert.NotEmpty(t, tx.CompletedAt)

		err = monitorService.SyncTransaction(ctx, &schemas.EventPaymentCompletedData{TransactionID: tx.ID})
		assert.ErrorContains(t, err, fmt.Sprintf("expected exactly 1 payment for the transaction ID %s but found 0", tx.ID))
	})
}

func createTSSTxs(t *testing.T, testCtx *testContext, payments ...*data.Payment) []*txSubStore.Transaction {
	t.Helper()

	paymentsQuantity := len(payments)
	transactionsToCreate := make([]txSubStore.Transaction, 0, paymentsQuantity)
	for _, payment := range payments {
		amount, err := strconv.ParseFloat(payment.Amount, 64)
		require.NoError(t, err)

		transactionsToCreate = append(transactionsToCreate, txSubStore.Transaction{
			ExternalID:  payment.ID,
			AssetCode:   payment.Asset.Code,
			AssetIssuer: payment.Asset.Issuer,
			Amount:      amount,
			Destination: payment.ReceiverWallet.StellarAddress,
			TenantID:    testCtx.tenantID,
		})
	}

	transactionsCreated, err := testCtx.tssModel.BulkInsert(testCtx.ctx, testCtx.tssModel.DBConnectionPool, transactionsToCreate)
	require.NoError(t, err)
	require.Len(t, transactionsCreated, paymentsQuantity)

	n, err := testCtx.sdpModel.Payment.UpdateStatuses(testCtx.ctx, testCtx.sdpModel.DBConnectionPool, payments, data.PendingPaymentStatus)
	require.NoError(t, err)
	require.Equal(t, int64(paymentsQuantity), n)

	transactions := make([]*txSubStore.Transaction, 0, paymentsQuantity)
	for i := range transactionsCreated {
		transactions = append(transactions, &transactionsCreated[i])
	}

	return transactions
}

func prepareTxsForSync(t *testing.T, testCtx *testContext, transactions []*txSubStore.Transaction) {
	t.Helper()

	txLen := len(transactions)

	var err error

	for _, tx := range transactions {
		q := `UPDATE submitter_transactions SET stellar_transaction_hash = $1, status=$2 WHERE id = $3`
		_, err = testCtx.tssModel.DBConnectionPool.ExecContext(testCtx.ctx, q, "test-hash-"+tx.ID, txSubStore.TransactionStatusProcessing, tx.ID)
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

	unsyncTransactions(t, testCtx, transactionIDs)

	// Set payment status back to pending
	for _, tx := range transactions {
		updatePaymentStatus(t, testCtx, tx.ExternalID, data.PendingPaymentStatus)
	}
}

func updatePaymentStatus(t *testing.T, testCtx *testContext, paymentID string, status data.PaymentStatus) {
	t.Helper()

	query := `UPDATE payments SET status = $1 WHERE id = $2`
	result, err := testCtx.sdpModel.DBConnectionPool.ExecContext(testCtx.ctx, query, status, paymentID)
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), rowsAffected)
}

func unsyncTransactions(t *testing.T, testCtx *testContext, transactionIDs []string) {
	t.Helper()

	query := `UPDATE submitter_transactions SET synced_at = NULL WHERE id = ANY($1)`
	_, err := testCtx.sdpModel.DBConnectionPool.ExecContext(testCtx.ctx, query, pq.Array(transactionIDs))
	require.NoError(t, err)
}

type payloadToUpdateTSSTxToError struct {
	transactionID  string
	statusMessages string
}

func updateTSSTransactionsToError(t *testing.T, testCtx *testContext, txDataSlice []payloadToUpdateTSSTxToError) []txSubStore.Transaction {
	t.Helper()

	var transactionIDs []string
	var statusMessages []sql.NullString
	for _, txData := range txDataSlice {
		transactionIDs = append(transactionIDs, txData.transactionID)
		statusMessages = append(statusMessages, sql.NullString{String: txData.statusMessages, Valid: txData.statusMessages != ""})
	}

	updatedTransactions := []txSubStore.Transaction{}
	// First update and get IDs
	q := `
		UPDATE submitter_transactions t
		SET
			status = $1,
			status_message = u.status_message,
			completed_at = NOW()
		FROM (SELECT UNNEST($2::text[]) as id, UNNEST($3::text[]) as status_message) as u
		WHERE t.id = u.id
		RETURNING ` + txSubStore.TransactionColumnNames("t", "")
	err := testCtx.sdpModel.DBConnectionPool.SelectContext(testCtx.ctx, &updatedTransactions, q, txSubStore.TransactionStatusError, pq.Array(transactionIDs), pq.Array(statusMessages))
	require.NoError(t, err)

	return updatedTransactions
}

func Test_PaymentFromSubmitterService_RetryingPayment(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	monitorService := NewPaymentFromSubmitterService(testCtx.sdpModel, dbConnectionPool)

	// create fixtures
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Disbursements, &data.Disbursement{
		Name:   "started disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		Amount:               "100",
		StellarTransactionID: "stellar-transaction-id-1",
		StellarOperationID:   "operation-id-1",
		Status:               data.ReadyPaymentStatus,
		Disbursement:         disbursement,
		ReceiverWallet:       receiverWallet,
		Asset:                *asset,
	})

	transactions := createTSSTxs(t, testCtx, payment)
	require.Len(t, transactions, 1)

	paymentDB, err := testCtx.sdpModel.Payment.Get(ctx, payment.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.PendingPaymentStatus, paymentDB.Status)

	transaction := transactions[0]
	assert.Equal(t, payment.ID, transaction.ExternalID)
	assert.Equal(t, txSubStore.TransactionStatusPending, transaction.Status)

	// GIVEN a payment that fails to be sent
	prepareTxsForSync(t, testCtx, transactions)
	updatedTransaction := updateTSSTransactionsToError(t, testCtx, []payloadToUpdateTSSTxToError{
		{transactionID: transaction.ID, statusMessages: "Failing Test"},
	})
	require.Len(t, updatedTransaction, 1)
	transaction = &updatedTransaction[0]
	assert.Equal(t, payment.ID, transaction.ExternalID)
	assert.Equal(t, txSubStore.TransactionStatusError, transaction.Status)

	// WHEN the monitor service is called
	err = monitorService.SyncTransaction(ctx, &schemas.EventPaymentCompletedData{
		TransactionID: transaction.ID,
	})
	require.NoError(t, err)

	// THEN the payment is synced to the error state
	paymentDB, err = testCtx.sdpModel.Payment.Get(ctx, paymentDB.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.FailedPaymentStatus, paymentDB.Status)
	assert.Len(t, paymentDB.StatusHistory, 3)
	assert.Equal(t, data.FailedPaymentStatus, paymentDB.StatusHistory[2].Status)
	assert.Equal(t, "Failing Test", paymentDB.StatusHistory[2].StatusMessage)

	// AND the payment is retried
	err = testCtx.sdpModel.Payment.RetryFailedPayments(ctx, dbConnectionPool, "email@test.com", paymentDB.ID)
	require.NoError(t, err)

	paymentDB, err = testCtx.sdpModel.Payment.Get(ctx, paymentDB.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.ReadyPaymentStatus, paymentDB.Status)

	// AND a new transaction is created for the payment
	transactions = createTSSTxs(t, testCtx, payment)
	require.Len(t, transactions, 1)

	paymentDB, err = testCtx.sdpModel.Payment.Get(ctx, payment.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.PendingPaymentStatus, paymentDB.Status)

	transactions, err = testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{payment.ID})
	require.NoError(t, err)
	require.Len(t, transactions, 2)

	transaction1 := transactions[0]
	transaction2 := transactions[1]
	assert.Equal(t, txSubStore.TransactionStatusError, transaction1.Status)
	assert.Equal(t, txSubStore.TransactionStatusPending, transaction2.Status)

	prepareTxsForSync(t, testCtx, transactions[1:])
	transaction2, err = testCtx.tssModel.Get(ctx, transaction2.ID)
	require.NoError(t, err)
	assert.Equal(t, txSubStore.TransactionStatusSuccess, transaction2.Status)

	err = monitorService.SyncTransaction(ctx, &schemas.EventPaymentCompletedData{
		TransactionID: transaction2.ID,
	})
	require.NoError(t, err)

	paymentDB, err = testCtx.sdpModel.Payment.Get(ctx, paymentDB.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.SuccessPaymentStatus, paymentDB.Status)
	assert.Len(t, paymentDB.StatusHistory, 6)
	assert.Equal(t, data.SuccessPaymentStatus, paymentDB.StatusHistory[5].Status)
	assert.Empty(t, paymentDB.StatusHistory[5].StatusMessage)
}

func Test_PaymentFromSubmitterService_CompleteDisbursements(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	testCtx := setupTestContext(t, dbConnectionPool)
	ctx := testCtx.ctx

	monitorService := NewPaymentFromSubmitterService(testCtx.sdpModel, dbConnectionPool)

	// create fixtures
	wallet := data.CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet", "https://www.wallet.com", "www.wallet.com", "wallet://")
	asset := data.CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GABC65XJDMXTGPNZRCI6V3KOKKWVK55UEKGQLONRIVYPMEJNNQ45YOEE")

	receiver := data.CreateReceiverFixture(t, ctx, dbConnectionPool, &data.Receiver{})
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbConnectionPool, receiver.ID, wallet.ID, data.RegisteredReceiversWalletStatus)

	disbursement := data.CreateDisbursementFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Disbursements, &data.Disbursement{
		Name:   "started disbursement",
		Status: data.StartedDisbursementStatus,
		Asset:  asset,
		Wallet: wallet,
	})

	payment := data.CreatePaymentFixture(t, ctx, dbConnectionPool, testCtx.sdpModel.Payment, &data.Payment{
		Amount:               "100",
		StellarTransactionID: "stellar-transaction-id-2",
		StellarOperationID:   "operation-id-2",
		Status:               data.ReadyPaymentStatus,
		Disbursement:         disbursement,
		ReceiverWallet:       receiverWallet,
		Asset:                *asset,
	})

	transactions := createTSSTxs(t, testCtx, payment)
	require.Len(t, transactions, 1)

	paymentDB, err := testCtx.sdpModel.Payment.Get(ctx, payment.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.PendingPaymentStatus, paymentDB.Status)

	transaction := transactions[0]
	assert.Equal(t, payment.ID, transaction.ExternalID)
	assert.Equal(t, txSubStore.TransactionStatusPending, transaction.Status)

	// GIVEN a payment that fails to be sent
	prepareTxsForSync(t, testCtx, transactions)
	updatedTransaction := updateTSSTransactionsToError(t, testCtx, []payloadToUpdateTSSTxToError{
		{transactionID: transaction.ID, statusMessages: "Failing Test"},
	})
	require.Len(t, updatedTransaction, 1)
	transaction = &updatedTransaction[0]
	assert.Equal(t, payment.ID, transaction.ExternalID)
	assert.Equal(t, txSubStore.TransactionStatusError, transaction.Status)

	// WHEN the monitor service is called
	err = monitorService.SyncTransaction(testCtx.ctx, &schemas.EventPaymentCompletedData{TransactionID: transaction.ID})
	require.NoError(t, err)

	// THEN the disbursement will not be completed
	disbursement, err = testCtx.sdpModel.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
	require.NoError(t, err)
	assert.Equal(t, data.StartedDisbursementStatus, disbursement.Status)

	// AND the payment is retried
	err = testCtx.sdpModel.Payment.RetryFailedPayments(ctx, dbConnectionPool, "email@test.com", paymentDB.ID)
	require.NoError(t, err)

	paymentDB, err = testCtx.sdpModel.Payment.Get(ctx, paymentDB.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.ReadyPaymentStatus, paymentDB.Status)

	// AND a new transaction is created for the payment
	createTSSTxs(t, testCtx, paymentDB)

	paymentDB, err = testCtx.sdpModel.Payment.Get(ctx, payment.ID, dbConnectionPool)
	require.NoError(t, err)
	assert.Equal(t, data.PendingPaymentStatus, paymentDB.Status)

	transactions, err = testCtx.tssModel.GetAllByPaymentIDs(ctx, []string{payment.ID})
	require.NoError(t, err)
	require.Len(t, transactions, 2)

	transaction1 := transactions[0]
	transaction2 := transactions[1]
	assert.Equal(t, txSubStore.TransactionStatusError, transaction1.Status)
	assert.Equal(t, txSubStore.TransactionStatusPending, transaction2.Status)

	prepareTxsForSync(t, testCtx, transactions[1:])
	transaction2, err = testCtx.tssModel.Get(ctx, transaction2.ID)
	require.NoError(t, err)
	assert.Equal(t, txSubStore.TransactionStatusSuccess, transaction2.Status)

	// WHEN the monitor service is called again
	err = monitorService.SyncTransaction(testCtx.ctx, &schemas.EventPaymentCompletedData{TransactionID: transaction2.ID})
	require.NoError(t, err)

	// THEN disbursement gets completed
	disbursement, err = testCtx.sdpModel.Disbursements.Get(ctx, dbConnectionPool, disbursement.ID)
	require.NoError(t, err)
	assert.Equal(t, data.CompletedDisbursementStatus, disbursement.Status)
}
