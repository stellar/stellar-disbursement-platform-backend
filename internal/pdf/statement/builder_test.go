package statement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

// TestBuildPDF renders one statement per layout path and checks each builds; the numbers on the
// page come straight from the service, whose tests pin them.
func TestBuildPDF(t *testing.T) {
	credit := services.StatementTransaction{ID: "tx1", CreatedAt: "2026-01-15T10:00:00Z", Type: "credit", Amount: "100.0000000", CounterpartyAddress: "GABCDEF123456789", CounterpartyName: "Test Counterparty"}
	debit := services.StatementTransaction{ID: "tx2", CreatedAt: "2026-01-16T10:00:00Z", Type: "debit", Amount: "50.0000000", CounterpartyAddress: "GZYXWV987654321", ExternalPaymentID: "ext-1"}
	xlm := func(beginning string, txs ...services.StatementTransaction) services.StatementAssetSummary {
		return services.StatementAssetSummary{Code: "XLM", BeginningBalance: beginning, TotalCredits: "100.0000000", TotalDebits: "50.0000000", EndingBalance: "50.0000000", Reconciled: true, Transactions: txs}
	}
	usdc := services.StatementAssetSummary{Code: "USDC", BeginningBalance: "0.0000000", TotalCredits: "0.0000000", TotalDebits: "0.0000000", EndingBalance: "0.0000000", Reconciled: true}
	statement := func(account string, assets ...services.StatementAssetSummary) *services.StatementResult {
		return &services.StatementResult{Summary: services.StatementSummary{Account: account, AccountName: "Main account", Assets: assets}}
	}
	notReconciled := xlm("0.0000000", credit)
	notReconciled.Reconciled = false

	testCases := []struct {
		name   string
		result *services.StatementResult
		logo   []byte
	}{
		{name: "credit and debit rows", result: statement("GABC", xlm("0.0000000", credit, debit))},
		{name: "no transactions", result: statement("GABC", xlm("0.0000000"))},
		{name: "several assets", result: statement("GABC", xlm("0.0000000", credit), usdc)},
		{name: "stellar: prefix on the account", result: statement("stellar:GABC", xlm("0.0000000", credit))},
		{name: "unparseable beginning balance still renders", result: statement("GABC", xlm("invalid-balance", credit))},
		{name: "not reconciled", result: statement("GABC", notReconciled)},
		{name: "organization logo", result: statement("GABC", xlm("0.0000000")), logo: []byte{0x89, 0x50, 0x4E, 0x47}},
	}
	from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pdfBytes, err := BuildPDF(tc.result, from, to, "Test Organization", tc.logo, "example.com", "https://stellar.expert/explorer/testnet/")
			require.NoError(t, err)
			assert.Greater(t, len(pdfBytes), 1000)
		})
	}
}
