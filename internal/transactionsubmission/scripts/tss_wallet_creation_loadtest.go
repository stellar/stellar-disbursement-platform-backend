//go:build wallet_loadtest

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

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

// calculateAndPrintWalletMetrics gets the transaction details from Horizon and calculates and prints wallet creation metrics.
func calculateAndPrintWalletMetrics(ctx context.Context, horizonClient *horizonclient.Client, txModel *store.TransactionModel, transactionIDs []string) {
	transactionsTSS := make(map[string]*store.Transaction)
	transactionsStellar := make(map[string]*horizon.Transaction)
	transactionLatencies := make([]time.Duration, 0, len(transactionIDs))
	uniqueLedgers := make(map[int32]bool)
	for _, transactionID := range transactionIDs {
		tx, _ := txModel.Get(ctx, transactionID)
		transactionsTSS[transactionID] = tx
	}

	for txnId, txn := range transactionsTSS {
		stellarTxn, err := horizonClient.TransactionDetail(txn.StellarTransactionHash.String)
		// time.Sleep(100000) might need to sleep if getting rate limited
		if err != nil {
			fmt.Printf("failed to retrieve stellar transaction %s from horizon", txn.StellarTransactionHash.String)
		}
		transactionsStellar[txnId] = &stellarTxn
	}

	minCreatedWalletTime := time.Now()
	for _, tx := range transactionsTSS {
		if tx.CreatedAt.Before(minCreatedWalletTime) {
			minCreatedWalletTime = *tx.CreatedAt
		}
	}

	maxCreatedWalletTime := minCreatedWalletTime
	for _, tx := range transactionsTSS {
		if tx.CreatedAt.After(maxCreatedWalletTime) {
			maxCreatedWalletTime = *tx.CreatedAt
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
	maxTxnLatency := time.Duration(math.MinInt64)
	for _, txId := range transactionIDs {
		start := transactionsTSS[txId].CreatedAt
		finish := transactionsStellar[txId].LedgerCloseTime
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

	fmt.Printf("Test size: %d wallet creation(s)\n", len(transactionIDs))
	fmt.Printf("==========================================================\n")
	fmt.Printf("TSS first created wallet time:       %s\n", minCreatedWalletTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("Stellar first observed wallet time:  %s\n", minStellarTxnCreatedTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("TSS last created wallet time:        %s\n", maxCreatedWalletTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("Stellar final wallet observed time:  %s\n", maxStellarTxnCreatedTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("=========================================================\n")
	fmt.Printf("Total test latency (first created, last observed): %.2fs\n", maxStellarTxnCreatedTime.Sub(minCreatedWalletTime).Seconds())
	fmt.Printf("==========================================================\n")
	fmt.Printf("min e2e wallet creation latency:     %.2fs\n", minTxnLatency.Seconds())
	fmt.Printf("average e2e wallet creation latency: %.2fs\n", avgLatency.Seconds())
	fmt.Printf("max e2e wallet creation latency:     %.2fs\n", maxTxnLatency.Seconds())
	fmt.Printf("median e2e wallet creation latency:  %.2fs\n", medianLatency.Seconds())
	fmt.Printf("==========================================================\n")
	totalDuration := maxStellarTxnCreatedTime.Sub(minCreatedWalletTime).Seconds()
	var walletsPerSec float64
	if totalDuration > 0 && len(transactionIDs) > 0 {
		walletsPerSec = float64(len(transactionIDs)) / totalDuration
	}
	fmt.Printf("calculated average wallets/sec:      %.2f\n", walletsPerSec)
	fmt.Printf("unique ledgers:                      %d\n", len(uniqueLedgers))
	fmt.Printf("==========================================================\n\n")

	fmt.Printf("%.2f, %d, %d, %d, %d, %s, %s, %.2f, %.2f, %.2f, %.2f, %.2f\n\n",
		walletsPerSec,
		0,
		0,
		0,
		len(uniqueLedgers),
		minCreatedWalletTime.In(time.Local).Format("2006-01-02 15:04:05"),
		minStellarTxnCreatedTime.In(time.Local).Format("2006-01-02 15:04:05"),
		maxStellarTxnCreatedTime.Sub(minCreatedWalletTime).Seconds(),
		minTxnLatency.Seconds(),
		medianLatency.Seconds(),
		avgLatency.Seconds(),
		maxTxnLatency.Seconds(),
	)
}

// generateUniquePublicKey generates a unique 65-byte (130 hex character) public key for wallet creation.
// Each public key must be unique as it serves as the contract deployment salt.
func generateUniquePublicKey() (string, error) {
	// Generate 64 random bytes and prepend with '04' to make it 65 bytes total
	randomBytes := make([]byte, 64)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Prepend with '04' to indicate uncompressed public key format (65 bytes total)
	publicKey := "04" + hex.EncodeToString(randomBytes)
	return publicKey, nil
}

// createWalletCreationTransactions creates bulk wallet creation transactions in the submitter_transactions table for TSS to process.
func createWalletCreationTransactions(ctx context.Context, txModel *store.TransactionModel, walletCount int, wasmHash, tenantId string) []store.Transaction {
	transactions := make([]store.Transaction, 0, walletCount)
	for i := 0; i < walletCount; i++ {
		externalID := fmt.Sprintf("wallet_loadtest_%d_%03d", time.Now().Unix(), i+1)

		// Generate unique public key for each wallet creation
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
			TenantID: tenantId,
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

// waitForWalletCreationTransactionsToComplete queries the database for each transaction that was created as waits for all of them to
// be in either SUCCESS or ERROR state.
func waitForWalletCreationTransactionsToComplete(ctx context.Context, txModel *store.TransactionModel, transactionIDs []string) {
	ticker := time.NewTicker(10 * time.Second)
	tickerChan := ticker.C
	for range tickerChan {
		completedTransactions := 0
		successfulTransactions := 0
		errorTransactions := 0

		for _, transactionID := range transactionIDs {
			// TODO - optimize this into a single query to get all statuses
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
			// All transactions are complete, exit the loop
			ticker.Stop()
			fmt.Printf("All %d wallet creation transactions have completed!\n", len(transactionIDs))
			fmt.Printf("Success: %d, Errors: %d\n", successfulTransactions, errorTransactions)
			return
		}
		fmt.Printf("%d/%d wallet creation transactions have completed (Success: %d, Errors: %d)...\n",
			completedTransactions, len(transactionIDs), successfulTransactions, errorTransactions)
	}
}

// This script is meant for creating a large number of wallet creation transactions for TESTING.
// There is minimal error handling and minimal checking for valid input parameters.
func main() {
	walletCount := flag.Int("walletCount", 0, "how many wallet creation transactions to create")
	databaseUrl := flag.String("databaseUrl", "", "database to create the transactions in")
	horizonUrl := flag.String("horizonUrl", "https://horizon-testnet.stellar.org", "horizon url")
	wasmHash := flag.String("wasmHash", "a5016f845e76fe452de6d3638ac47523b845a813db56de3d713eb7a49276e254", "WASM hash for contract deployment")
	tenantId := flag.String("tenantId", "", "tenant ID for multi-tenant testing (optional)")
	flag.Parse()

	if *walletCount <= 0 {
		fmt.Printf("Error: walletCount must be greater than 0\n")
		return
	}

	if *databaseUrl == "" {
		fmt.Printf("Error: databaseUrl is required\n")
		return
	}

	ctx := context.Background()
	dbConnectionPool, err := db.OpenDBConnectionPool(*databaseUrl)
	if err != nil {
		fmt.Printf("Error opening db connection pool in init: %s ", err.Error())
		return
	}

	txModel := &store.TransactionModel{DBConnectionPool: dbConnectionPool}

	// create horizon client
	horizonClient := &horizonclient.Client{
		HorizonURL: *horizonUrl,
		HTTP:       httpclient.DefaultClient(),
	}

	fmt.Printf("Starting wallet creation load test with %d transactions\n", *walletCount)
	fmt.Printf("WASM Hash: %s\n", *wasmHash)
	if *tenantId != "" {
		fmt.Printf("Tenant ID: %s\n", *tenantId)
	}
	fmt.Printf("==========================================================\n")

	// 1) create the wallet creation transactions
	transactionIDs := createWalletCreationTransactions(ctx, txModel, *walletCount, *wasmHash, *tenantId)
	txIDs := make([]string, 0, len(transactionIDs))
	for _, tx := range transactionIDs {
		txIDs = append(txIDs, tx.ID)
	}

	// 2) wait for all Transactions to be marked as either Success/Error
	waitForWalletCreationTransactionsToComplete(ctx, txModel, txIDs)

	// 3) calculate and print wallet creation metrics
	calculateAndPrintWalletMetrics(ctx, horizonClient, txModel, txIDs)
}
