package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

type TestType string

const (
	TestTypePayment        TestType = "payment"
	TestTypeWalletCreation TestType = "wallet"
)

// calculateAndPrintMetrics gets the transaction details from Horizon and calculates and prints metrics.
func calculateAndPrintMetrics(ctx context.Context, horizonClient *horizonclient.Client, txModel *store.TransactionModel, transactionIDs []string, testType TestType) {
	transactionsTSS := make(map[string]*store.Transaction)
	transactionsStellar := make(map[string]*horizon.Transaction)
	transactionLatencies := make([]time.Duration, 0, len(transactionIDs))
	uniqueLedgers := make(map[int32]bool)

	for _, transactionID := range transactionIDs {
		tx, err := txModel.Get(ctx, transactionID)
		if err != nil {
			fmt.Printf("failed to get transaction %s from tss", transactionID)
			continue
		}
		transactionsTSS[transactionID] = tx
	}

	for txnID, txn := range transactionsTSS {
		stellarTxn, err := horizonClient.TransactionDetail(txn.StellarTransactionHash.String)
		if err != nil {
			fmt.Printf("failed to retrieve stellar transaction %s from horizon: %v\n", txn.StellarTransactionHash.String, err)
			continue
		}
		transactionsStellar[txnID] = &stellarTxn
	}

	if len(transactionsTSS) == 0 || len(transactionsStellar) == 0 {
		fmt.Printf("No transactions to analyze\n")
		return
	}

	minCreatedTime := time.Now()
	maxCreatedTime := time.Time{}
	for _, tx := range transactionsTSS {
		if tx.CreatedAt.Before(minCreatedTime) {
			minCreatedTime = *tx.CreatedAt
		}
		if tx.CreatedAt.After(maxCreatedTime) {
			maxCreatedTime = *tx.CreatedAt
		}
	}

	minStellarTxnCreatedTime := time.Now()
	maxStellarTxnCreatedTime := time.Time{}
	for _, tx := range transactionsStellar {
		if tx.LedgerCloseTime.Before(minStellarTxnCreatedTime) {
			minStellarTxnCreatedTime = tx.LedgerCloseTime
		}
		if tx.LedgerCloseTime.After(maxStellarTxnCreatedTime) {
			maxStellarTxnCreatedTime = tx.LedgerCloseTime
		}
		uniqueLedgers[tx.Ledger] = true
	}

	minTxnLatency := time.Duration(math.MaxInt64)
	maxTxnLatency := time.Duration(0)
	for _, txID := range transactionIDs {
		tssTransaction, exists := transactionsTSS[txID]
		if !exists {
			continue
		}
		stellarTransaction, exists := transactionsStellar[txID]
		if !exists {
			continue
		}

		start := tssTransaction.CreatedAt
		finish := stellarTransaction.LedgerCloseTime
		duration := finish.Sub(*start)
		transactionLatencies = append(transactionLatencies, duration)
		if duration < minTxnLatency {
			minTxnLatency = duration
		}
		if duration > maxTxnLatency {
			maxTxnLatency = duration
		}
	}

	var avgLatency time.Duration
	if len(transactionLatencies) > 0 {
		sumLatency := time.Duration(0)
		for _, duration := range transactionLatencies {
			sumLatency += duration
		}
		avgLatency = sumLatency / time.Duration(len(transactionLatencies))
	}

	var medianLatency time.Duration
	if len(transactionLatencies) > 0 {
		sort.Slice(transactionLatencies, func(i, j int) bool {
			return transactionLatencies[i] < transactionLatencies[j]
		})
		mid := len(transactionLatencies) / 2
		medianLatency = transactionLatencies[mid]
	}

	testName := "transaction"
	switch testType {
	case TestTypePayment:
		testName = "payment"
	case TestTypeWalletCreation:
		testName = "wallet creation"
	}

	fmt.Printf("Test size: %d %s(s)\n", len(transactionIDs), testName)
	fmt.Printf("==========================================================\n")
	fmt.Printf("TSS first created %s time:       %s\n", testName, minCreatedTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("Stellar first observed %s time:  %s\n", testName, minStellarTxnCreatedTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("TSS last created %s time:        %s\n", testName, maxCreatedTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("Stellar final %s observed time:  %s\n", testName, maxStellarTxnCreatedTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("=========================================================\n")
	fmt.Printf("Total test latency (first created, last observed): %.2fs\n", maxStellarTxnCreatedTime.Sub(minCreatedTime).Seconds())
	fmt.Printf("==========================================================\n")
	fmt.Printf("min e2e %s latency:     %.2fs\n", testName, minTxnLatency.Seconds())
	fmt.Printf("average e2e %s latency: %.2fs\n", testName, avgLatency.Seconds())
	fmt.Printf("max e2e %s latency:     %.2fs\n", testName, maxTxnLatency.Seconds())
	fmt.Printf("median e2e %s latency:  %.2fs\n", testName, medianLatency.Seconds())
	fmt.Printf("==========================================================\n")

	totalDuration := maxStellarTxnCreatedTime.Sub(minCreatedTime).Seconds()
	var tps float64
	if totalDuration > 0 && len(transactionIDs) > 0 {
		tps = float64(len(transactionIDs)) / totalDuration
	}

	tpsLabel := "TPS"
	if testType == TestTypeWalletCreation {
		tpsLabel = "wallets/sec"
	}
	fmt.Printf("calculated average %s:      %.2f\n", tpsLabel, tps)
	fmt.Printf("unique ledgers:              %d\n", len(uniqueLedgers))
	fmt.Printf("==========================================================\n\n")

	fmt.Printf("%.2f, %d, %d, %d, %d, %s, %s, %.2f, %.2f, %.2f, %.2f, %.2f\n\n",
		tps,
		0, 0, 0,
		len(uniqueLedgers),
		minCreatedTime.In(time.Local).Format("2006-01-02 15:04:05"),
		minStellarTxnCreatedTime.In(time.Local).Format("2006-01-02 15:04:05"),
		maxStellarTxnCreatedTime.Sub(minCreatedTime).Seconds(),
		minTxnLatency.Seconds(),
		medianLatency.Seconds(),
		avgLatency.Seconds(),
		maxTxnLatency.Seconds(),
	)
}

// generateUniquePublicKey generates a unique 65-byte (130 hex character) public key for wallet creation.
func generateUniquePublicKey() (string, error) {
	randomBytes := make([]byte, 64)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	publicKey := "04" + hex.EncodeToString(randomBytes)
	return publicKey, nil
}

// createPaymentTransactions creates bulk payment transactions in the submitter_transactions table for TSS to process.
func createPaymentTransactions(ctx context.Context, txModel *store.TransactionModel, paymentCount int, assetCode, assetIssuer, destination, tenantID string) []store.Transaction {
	transactions := make([]store.Transaction, 0, paymentCount)
	for i := range paymentCount {
		externalID := fmt.Sprintf("payment_loadtest_%d_%03d", time.Now().Unix(), i+1)
		transactions = append(transactions, store.Transaction{
			ExternalID:      externalID,
			TransactionType: store.TransactionTypePayment,
			Payment: store.Payment{
				AssetCode:   assetCode,
				AssetIssuer: assetIssuer,
				Amount:      decimal.NewFromFloat(0.1),
				Destination: destination,
			},
			TenantID: tenantID,
		})
	}

	insertedTransactions, err := txModel.BulkInsert(ctx, txModel.DBConnectionPool, transactions)
	if err != nil {
		log.Ctx(ctx).Errorf("Error inserting payment transactions: %v", err.Error())
	}

	fmt.Printf("Successfully created %d payment transactions\n", len(insertedTransactions))
	return insertedTransactions
}

// createWalletCreationTransactions creates bulk wallet creation transactions in the submitter_transactions table for TSS to process.
func createWalletCreationTransactions(ctx context.Context, txModel *store.TransactionModel, walletCount int, wasmHash, tenantID string) []store.Transaction {
	transactions := make([]store.Transaction, 0, walletCount)
	for i := range walletCount {
		externalID := fmt.Sprintf("wallet_loadtest_%d_%03d", time.Now().Unix(), i+1)

		publicKey, err := generateUniquePublicKey()
		if err != nil {
			log.Ctx(ctx).Errorf("Error generating public key for transaction %d: %v", i, err)
			continue
		}

		transaction := store.Transaction{
			ExternalID:      externalID,
			TransactionType: store.TransactionTypeWalletCreation,
			WalletCreation: store.WalletCreation{
				PublicKey: publicKey,
				WasmHash:  wasmHash,
			},
			TenantID: tenantID,
		}

		transactions = append(transactions, transaction)
	}

	insertedTransactions, err := txModel.BulkInsert(ctx, txModel.DBConnectionPool, transactions)
	if err != nil {
		log.Ctx(ctx).Errorf("Error inserting wallet creation transactions: %v", err.Error())
	}

	fmt.Printf("Successfully created %d wallet creation transactions\n", len(insertedTransactions))
	return insertedTransactions
}

// waitForTransactionsToComplete queries the database for each transaction and waits for all to complete.
func waitForTransactionsToComplete(ctx context.Context, txModel *store.TransactionModel, transactionIDs []string, testType TestType) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	testName := "transaction"
	switch testType {
	case TestTypePayment:
		testName = "payment"
	case TestTypeWalletCreation:
		testName = "wallet creation"
	}

	for range ticker.C {
		completedTransactions := 0
		successfulTransactions := 0
		errorTransactions := 0

		for _, transactionID := range transactionIDs {
			tx, err := txModel.Get(ctx, transactionID)
			if err != nil {
				log.Ctx(ctx).Errorf("Error getting transaction %s: %v", transactionID, err)
			} else if tx.Status == store.TransactionStatusError {
				completedTransactions += 1
				errorTransactions += 1
			} else if tx.Status == store.TransactionStatusSuccess {
				completedTransactions += 1
				successfulTransactions += 1
			}
		}

		if completedTransactions == len(transactionIDs) {
			fmt.Printf("All %d %s transactions have completed!\n", len(transactionIDs), testName)
			fmt.Printf("Success: %d, Errors: %d\n", successfulTransactions, errorTransactions)
			return
		}

		fmt.Printf("%d/%d %s transactions have completed (Success: %d, Errors: %d)...\n",
			completedTransactions, len(transactionIDs), testName, successfulTransactions, errorTransactions)
	}
}

func main() {
	// Common flags
	testType := flag.String("type", "", "Type of load test: 'payment' or 'wallet' (required)")
	count := flag.Int("count", 0, "Number of transactions to create (required)")
	databaseURL := flag.String("databaseUrl", "", "Database connection URL (required)")
	horizonURL := flag.String("horizonUrl", "https://horizon-testnet.stellar.org", "Horizon server URL")
	tenantID := flag.String("tenantId", "", "Tenant ID for multi-tenant testing (required)")

	// Payment-specific flags
	assetCode := flag.String("assetCode", "USDC", "Asset code for payments")
	assetIssuer := flag.String("assetIssuer", "GDQOE23CFSUMSVQK4Y5JHPPYK73VYCNHZHA7ENKCV37P6SUEO6XQBKPP", "Asset issuer for payments")
	paymentDestination := flag.String("paymentDestination", "", "Destination address for payments (required for payment tests)")

	// Wallet creation-specific flags
	wasmHash := flag.String("wasmHash", "a5016f845e76fe452de6d3638ac47523b845a813db56de3d713eb7a49276e254", "WASM hash for wallet creation")

	flag.Parse()

	// Validate required parameters
	if *testType == "" {
		fmt.Printf("Error: -type is required. Use 'payment' or 'wallet'\n")
		flag.Usage()
		return
	}

	if *testType != "payment" && *testType != "wallet" {
		fmt.Printf("Error: -type must be 'payment' or 'wallet'\n")
		flag.Usage()
		return
	}

	if *count <= 0 {
		fmt.Printf("Error: -count must be greater than 0\n")
		return
	}

	if *databaseURL == "" {
		fmt.Printf("Error: -databaseUrl is required\n")
		return
	}

	if *tenantID == "" {
		fmt.Printf("Error: -tenantId is required\n")
		return
	}

	if *testType == "payment" && *paymentDestination == "" {
		fmt.Printf("Error: -paymentDestination is required for payment tests\n")
		return
	}

	ctx := context.Background()
	dbConnectionPool, err := db.OpenDBConnectionPool(*databaseURL)
	if err != nil {
		fmt.Printf("Error opening db connection pool: %s\n", err.Error())
		return
	}

	txModel := &store.TransactionModel{DBConnectionPool: dbConnectionPool}

	horizonClient := &horizonclient.Client{
		HorizonURL: *horizonURL,
		HTTP:       httpclient.DefaultClient(),
	}

	var testTypeEnum TestType
	var transactionIDs []store.Transaction

	if *testType == "payment" {
		testTypeEnum = TestTypePayment
		fmt.Printf("Starting payment load test with %d transactions\n", *count)
		fmt.Printf("Asset: %s:%s\n", *assetCode, *assetIssuer)
		fmt.Printf("Destination: %s\n", *paymentDestination)
		fmt.Printf("Tenant ID: %s\n", *tenantID)
		fmt.Printf("==========================================================\n")

		transactionIDs = createPaymentTransactions(ctx, txModel, *count, *assetCode, *assetIssuer, *paymentDestination, *tenantID)
	} else {
		testTypeEnum = TestTypeWalletCreation
		fmt.Printf("Starting wallet creation load test with %d transactions\n", *count)
		fmt.Printf("WASM Hash: %s\n", *wasmHash)
		fmt.Printf("Tenant ID: %s\n", *tenantID)
		fmt.Printf("==========================================================\n")

		transactionIDs = createWalletCreationTransactions(ctx, txModel, *count, *wasmHash, *tenantID)
	}

	txIDs := make([]string, 0, len(transactionIDs))
	for _, tx := range transactionIDs {
		txIDs = append(txIDs, tx.ID)
	}

	// Wait for all transactions to complete
	waitForTransactionsToComplete(ctx, txModel, txIDs, testTypeEnum)

	// Calculate and print metrics
	calculateAndPrintMetrics(ctx, horizonClient, txModel, txIDs, testTypeEnum)
}
