package statement

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

const (
	testAccountAddress = "stellar:GDNRRK5EXMZ4STV7UTO3CW4LSVNY5KYWTM3J7BM5SQNA7KE2RYX55IYV"
	testAmountZero     = "0.0000000"
	testAmount100      = "100.0000000"
	testAmount50       = "50.0000000"
	testTimestamp1     = "2026-01-15T10:00:00Z"
	testOrgName        = "Test Org"
)

func TestBuildPDF(t *testing.T) {
	t.Run("successfully generates PDF with transactions", func(t *testing.T) {
		result := &services.StatementResult{
			Summary: services.StatementSummary{
				Account: testAccountAddress,
				Assets: []services.StatementAssetSummary{
					{
						Code:             "XLM",
						BeginningBalance: testAmountZero,
						TotalCredits:     testAmount100,
						TotalDebits:      testAmount50,
						EndingBalance:    testAmount50,
						Transactions: []services.StatementTransaction{
							{
								ID:                  "tx1",
								CreatedAt:           testTimestamp1,
								Type:                "credit",
								Amount:              testAmount100,
								CounterpartyAddress: "GABCDEF123456789",
								CounterpartyName:    "Test Counterparty",
							},
							{
								ID:                  "tx2",
								CreatedAt:           "2026-01-16T10:00:00Z",
								Type:                "debit",
								Amount:              testAmount50,
								CounterpartyAddress: "GZYXWV987654321",
							},
						},
					},
				},
			},
		}

		fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
		orgName := "Test Organization"
		orgLogo := []byte{}

		pdfBytes, err := BuildPDF(result, fromDate, toDate, orgName, orgLogo, "")

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
		assert.Greater(t, len(pdfBytes), 1000, "PDF should be substantial size")
	})

	t.Run("handles empty transactions", func(t *testing.T) {
		result := &services.StatementResult{
			Summary: services.StatementSummary{
				Account: testAccountAddress,
				Assets: []services.StatementAssetSummary{
					{
						Code:             "XLM",
						BeginningBalance: testAmountZero,
						TotalCredits:     testAmountZero,
						TotalDebits:      testAmountZero,
						EndingBalance:    testAmountZero,
						Transactions:     []services.StatementTransaction{},
					},
				},
			},
		}

		fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		pdfBytes, err := BuildPDF(result, fromDate, toDate, testOrgName, nil, "")

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("handles multiple assets", func(t *testing.T) {
		result := &services.StatementResult{
			Summary: services.StatementSummary{
				Account: testAccountAddress,
				Assets: []services.StatementAssetSummary{
					{
						Code:             "XLM",
						BeginningBalance: testAmountZero,
						TotalCredits:     testAmount100,
						TotalDebits:      testAmountZero,
						EndingBalance:    testAmount100,
						Transactions: []services.StatementTransaction{
							{
								ID:        "tx1",
								CreatedAt: testTimestamp1,
								Type:      "credit",
								Amount:    testAmount100,
							},
						},
					},
					{
						Code:             "USDC",
						BeginningBalance: testAmountZero,
						TotalCredits:     testAmount50,
						TotalDebits:      testAmountZero,
						EndingBalance:    testAmount50,
						Transactions: []services.StatementTransaction{
							{
								ID:        "tx2",
								CreatedAt: "2026-01-16T10:00:00Z",
								Type:      "credit",
								Amount:    testAmount50,
							},
						},
					},
				},
			},
		}

		fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		pdfBytes, err := BuildPDF(result, fromDate, toDate, testOrgName, nil, "")

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("handles wallet address with stellar: prefix", func(t *testing.T) {
		result := &services.StatementResult{
			Summary: services.StatementSummary{
				Account: testAccountAddress,
				Assets: []services.StatementAssetSummary{
					{
						Code:             "XLM",
						BeginningBalance: testAmountZero,
						TotalCredits:     testAmountZero,
						TotalDebits:      testAmountZero,
						EndingBalance:    testAmountZero,
						Transactions:     []services.StatementTransaction{},
					},
				},
			},
		}

		fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		pdfBytes, err := BuildPDF(result, fromDate, toDate, testOrgName, nil, "")

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("handles invalid beginning balance gracefully", func(t *testing.T) {
		result := &services.StatementResult{
			Summary: services.StatementSummary{
				Account: testAccountAddress,
				Assets: []services.StatementAssetSummary{
					{
						Code:             "XLM",
						BeginningBalance: "invalid-balance",
						TotalCredits:     testAmount100,
						TotalDebits:      testAmountZero,
						EndingBalance:    testAmount100,
						Transactions: []services.StatementTransaction{
							{
								ID:        "tx1",
								CreatedAt: testTimestamp1,
								Type:      "credit",
								Amount:    testAmount100,
							},
						},
					},
				},
			},
		}

		fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

		pdfBytes, err := BuildPDF(result, fromDate, toDate, testOrgName, nil, "")

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})

	t.Run("handles organization logo", func(t *testing.T) {
		result := &services.StatementResult{
			Summary: services.StatementSummary{
				Account: testAccountAddress,
				Assets: []services.StatementAssetSummary{
					{
						Code:             "XLM",
						BeginningBalance: testAmountZero,
						TotalCredits:     testAmountZero,
						TotalDebits:      testAmountZero,
						EndingBalance:    testAmountZero,
						Transactions:     []services.StatementTransaction{},
					},
				},
			},
		}

		fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
		orgLogo := []byte{0x89, 0x50, 0x4E, 0x47} // PNG header

		pdfBytes, err := BuildPDF(result, fromDate, toDate, testOrgName, orgLogo, "")

		require.NoError(t, err)
		assert.NotEmpty(t, pdfBytes)
	})
}
