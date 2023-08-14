package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/store"
)

// calculateAndPrintMetrics gets the transaction details from Horizon and calculates and prints metrics.
func calculateAndPrintMetrics(ctx context.Context, horizonClient *horizonclient.Client, txModel *store.TransactionModel, transactionIDs []string) {
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

	minCreatedPaymentTime := time.Now()
	for _, tx := range transactionsTSS {
		if tx.CreatedAt.Before(minCreatedPaymentTime) {
			minCreatedPaymentTime = *tx.CreatedAt
		}
	}

	maxCreatedPaymentTime := minCreatedPaymentTime
	for _, tx := range transactionsTSS {
		if tx.CreatedAt.After(maxCreatedPaymentTime) {
			maxCreatedPaymentTime = *tx.CreatedAt
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

	sumLatency := time.Duration(0)
	for _, duration := range transactionLatencies {
		sumLatency += duration
	}
	avgLatency := sumLatency / time.Duration(len(transactionLatencies))

	sort.Slice(transactionLatencies, func(i, j int) bool {
		return transactionLatencies[i] < transactionLatencies[j]
	})
	mid := len(transactionLatencies) / 2
	medianLatency := transactionLatencies[mid]

	fmt.Printf("Test size: %d payment(s)\n", len(transactionIDs))
	fmt.Printf("==========================================================\n")
	fmt.Printf("TSS first created payment time:      %s\n", minCreatedPaymentTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("Stellar first observed payment time: %s\n", minStellarTxnCreatedTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("TSS last created payment time:       %s\n", maxCreatedPaymentTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("Stellar final payment observed time: %s\n", maxStellarTxnCreatedTime.In(time.Local).Format(time.Stamp))
	fmt.Printf("=========================================================\n")
	fmt.Printf("Total test latency (first created, last observed): %.2fs\n", maxStellarTxnCreatedTime.Sub(minCreatedPaymentTime).Seconds())
	fmt.Printf("==========================================================\n")
	fmt.Printf("min e2e payment latency:      %.2fs\n", minTxnLatency.Seconds())
	fmt.Printf("average e2e payment latency:  %.2fs\n", avgLatency.Seconds())
	fmt.Printf("max e2e payment latency:      %.2fs\n", maxTxnLatency.Seconds())
	fmt.Printf("==========================================================\n")
	fmt.Printf("calculated average TPS:       %.2f\n", float64(len(transactionIDs))/(maxStellarTxnCreatedTime.Sub(minCreatedPaymentTime).Seconds()))
	fmt.Printf("unique ledgers:               %d\n", len(uniqueLedgers))
	fmt.Printf("==========================================================\n\n")

	fmt.Printf("%.2f, %d, %d, %d, %d, %s, %s, %.2f, %.2f, %.2f, %.2f, %.2f\n\n",
		float64(len(transactionIDs))/(maxStellarTxnCreatedTime.Sub(minCreatedPaymentTime).Seconds()),
		0,
		0,
		0,
		len(uniqueLedgers),
		minCreatedPaymentTime.In(time.Local).Format("2006-01-02 15:04:05"),
		minStellarTxnCreatedTime.In(time.Local).Format("2006-01-02 15:04:05"),
		maxStellarTxnCreatedTime.Sub(minCreatedPaymentTime).Seconds(),
		minTxnLatency.Seconds(),
		medianLatency.Seconds(),
		avgLatency.Seconds(),
		maxTxnLatency.Seconds(),
	)
}

// createPaymentTransactions creates bulk transactions in the submitter_transactions table for TSS to process.
func createPaymentTransactions(ctx context.Context, txModel *store.TransactionModel, paymentCount int, assetCode, assetIssuer, destination string) []store.Transaction {
	transactions := make([]store.Transaction, 0, paymentCount)
	for i := 0; i < paymentCount; i++ {
		externalID := fmt.Sprintf("external-id-%d", i)
		transactions = append(transactions, store.Transaction{
			ExternalID:  externalID,
			AssetCode:   assetCode,
			AssetIssuer: assetIssuer,
			Amount:      0.1,
			Destination: destination,
		})
	}
	insertedTransactions, err := txModel.BulkInsert(ctx, txModel.DBConnectionPool, transactions)
	if err != nil {
		log.Ctx(ctx).Errorf("Error inserting transactions: %v", err.Error())
	}
	return insertedTransactions
}

// waitForTransactionsToComplete queries the database for each transaction that was created as waits for all of them to
// be in either SUCCESS or ERROR state.
func waitForTransactionsToComplete(ctx context.Context, txModel *store.TransactionModel, transactionIDs []string) {
	ticker := time.NewTicker(10 * time.Second)
	tickerChan := ticker.C
	for range tickerChan {
		completedTransactions := 0
		for _, transactionID := range transactionIDs {
			// TODO - optimize this into a single query to get all statuses
			tx, err := txModel.Get(ctx, transactionID)
			if err != nil {
				log.Ctx(ctx).Errorf("Error getting transaction %s: %v", transactionID, err)
			} else if tx.Status == store.TransactionStatusError || tx.Status == store.TransactionStatusSuccess {
				completedTransactions += 1
			}
		}

		if completedTransactions == len(transactionIDs) {
			// All transactions are complete, exit the loop
			ticker.Stop()
			fmt.Printf("All %d transactions have completed!\n", len(transactionIDs))
			return
		}
		fmt.Printf("%d/%d transactions have completed...\n", completedTransactions, len(transactionIDs))
	}
}

// This script is just meant for creating a large number of payments for TESTING.
// There is minimal error handling and minimal checking for valid input parameters.
func main() {
	paymentCount := flag.Int("paymentCount", 0, "how many payments to create")
	databaseUrl := flag.String("databaseUrl", "", "database to create the transactions in")
	horizonUrl := flag.String("horizonUrl", "https://horizon-testnet.stellar.org", "horizon url")
	assetCode := flag.String("assetCode", "USDC", "asset code")
	assetIssuer := flag.String("assetIssuer", "GDQOE23CFSUMSVQK4Y5JHPPYK73VYCNHZHA7ENKCV37P6SUEO6XQBKPP", "asset issuer")
	paymentDestination := flag.String("paymentDestination", "", "destination address of the payment")
	flag.Parse()

	ctx := context.Background()
	dbConnectionPool, err := db.OpenDBConnectionPool(*databaseUrl)
	if err != nil {
		fmt.Printf("Error opening db connection pool in init: %s ", err.Error())
	}

	txModel := &store.TransactionModel{DBConnectionPool: dbConnectionPool}

	// create horizon client
	horizonClient := &horizonclient.Client{
		HorizonURL: *horizonUrl,
		HTTP:       httpclient.DefaultClient(),
	}

	// 1) create the payment transactions
	transactionIDs := createPaymentTransactions(ctx, txModel, *paymentCount, *assetCode, *assetIssuer, *paymentDestination)
	txIDs := make([]string, 0, len(transactionIDs))
	for _, tx := range transactionIDs {
		txIDs = append(txIDs, tx.ID)
	}

	// 2) wait for all Transactions to be marked as either Success/Error
	waitForTransactionsToComplete(ctx, txModel, txIDs)

	// 3) calculate and print metrics
	calculateAndPrintMetrics(ctx, horizonClient, txModel, txIDs)
}
