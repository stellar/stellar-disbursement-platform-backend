package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/base"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/operations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func TestReportsServiceGetStatement(t *testing.T) {
	ctx := context.Background()
	accountAddress := keypair.MustRandom().Address()
	stellarAccount := schema.NewStellarEnvTransactionAccount(accountAddress)
	fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	t.Run("returns error for non-Stellar account", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		models := &data.Models{}

		service := NewReportsService(horizonClient, distSvc, models)
		nonStellarAccount := schema.TransactionAccount{
			Address: "circle:123",
			Type:    schema.DistributionAccountCircleDBVault,
		}

		result, err := service.GetStatement(ctx, &nonStellarAccount, "wallet-1", "", fromDate, toDate)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrStatementAccountNotStellar, err)
	})

	t.Run("returns error when asset not found", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		dbPool := testutils.GetDBConnectionPool(t)
		models, err := data.NewModels(dbPool)
		require.NoError(t, err)

		service := NewReportsService(horizonClient, distSvc, models)

		result, err := service.GetStatement(ctx, &stellarAccount, "wallet-1", "NONEXISTENT", fromDate, toDate)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, ErrStatementAssetNotFound, err)
	})

	t.Run("successfully gets statement for XLM asset", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		dbPool := testutils.GetDBConnectionPool(t)
		models, err := data.NewModels(dbPool)
		require.NoError(t, err)

		xlmAsset := data.Asset{Code: assets.XLMAssetCode, Issuer: ""}

		// Mock GetBalance
		distSvc.On("GetBalance", ctx, &stellarAccount, xlmAsset).
			Return(decimal.RequireFromString("100.0000000"), nil).Once()

		// Mock Payments - one scan, empty history
		var emptyPaymentsPage operations.OperationsPage
		emptyPaymentsPage.Embedded.Records = []operations.Operation{}
		horizonClient.On("Payments", mock.AnythingOfType("horizonclient.OperationRequest")).
			Return(emptyPaymentsPage, nil).Once()

		service := NewReportsService(horizonClient, distSvc, models)

		result, err := service.GetStatement(ctx, &stellarAccount, "wallet-1", "XLM", fromDate, toDate)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "stellar:"+accountAddress, result.Summary.Account)
		assert.Len(t, result.Summary.Assets, 1)
		assert.Equal(t, "XLM", result.Summary.Assets[0].Code)
	})

	t.Run("successfully gets statement for all assets", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		dbPool := testutils.GetDBConnectionPool(t)
		models, err := data.NewModels(dbPool)
		require.NoError(t, err)

		xlmAsset := data.Asset{Code: assets.XLMAssetCode, Issuer: ""}

		// Mock GetBalances
		distSvc.On("GetBalances", ctx, &stellarAccount).
			Return(map[data.Asset]decimal.Decimal{
				xlmAsset: decimal.RequireFromString("100.0000000"),
			}, nil).Once()

		// Mock GetBalance
		distSvc.On("GetBalance", ctx, &stellarAccount, xlmAsset).
			Return(decimal.RequireFromString("100.0000000"), nil).Once()

		// Mock Payments - one scan, empty history
		var emptyPaymentsPage operations.OperationsPage
		emptyPaymentsPage.Embedded.Records = []operations.Operation{}
		horizonClient.On("Payments", mock.AnythingOfType("horizonclient.OperationRequest")).
			Return(emptyPaymentsPage, nil).Once()

		service := NewReportsService(horizonClient, distSvc, models)

		result, err := service.GetStatement(ctx, &stellarAccount, "wallet-1", "", fromDate, toDate)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "stellar:"+accountAddress, result.Summary.Account)
		assert.Len(t, result.Summary.Assets, 1)
	})
}

func TestReportsServiceGetStatement_reconciliationAndTruncation(t *testing.T) {
	ctx := context.Background()
	accountAddress := keypair.MustRandom().Address()
	stellarAccount := schema.NewStellarEnvTransactionAccount(accountAddress)
	fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)
	xlmAsset := data.Asset{Code: assets.XLMAssetCode, Issuer: ""}
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	paymentOp := func(id, from, to, amount string, at time.Time) operations.Payment {
		op := operations.Payment{From: from, To: to, Amount: amount, Asset: base.Asset{Type: "native"}}
		op.Base.ID = id
		op.Base.PT = id
		op.Base.LedgerCloseTime = at
		return op
	}
	creditOp := func(id, amount string) operations.Payment {
		return paymentOp(id, "GSOMEONE", accountAddress, amount, time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC))
	}
	pageOf := func(ops ...operations.Operation) operations.OperationsPage {
		var page operations.OperationsPage
		page.Embedded.Records = ops
		return page
	}

	t.Run("a credit larger than today's balance cannot be reconciled", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		distSvc.On("GetBalance", ctx, &stellarAccount, xlmAsset).Return(decimal.RequireFromString("100"), nil).Once()
		horizonClient.On("Payments", mock.AnythingOfType("horizonclient.OperationRequest")).Return(pageOf(creditOp("1", "500")), nil).Once()

		result, err := NewReportsService(horizonClient, distSvc, models).GetStatement(ctx, &stellarAccount, "wallet-1", "XLM", fromDate, toDate)
		require.NoError(t, err)
		summary := result.Summary.Assets[0]
		assert.False(t, summary.Reconciled)
		assert.Equal(t, "0.0000000", summary.BeginningBalance)
		assert.Equal(t, "500.0000000", summary.TotalCredits)
	})

	t.Run("balances account for debits and for activity after the period", func(t *testing.T) {
		day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.UTC) }
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		distSvc.On("GetBalance", ctx, &stellarAccount, xlmAsset).Return(decimal.RequireFromString("1000"), nil).Once()
		// Newest first, as Horizon returns them. Every scan sees the same history, however many there are.
		horizonClient.On("Payments", mock.AnythingOfType("horizonclient.OperationRequest")).Return(pageOf(
			paymentOp("5", "GSOMEONE", accountAddress, "300", day(time.February, 10)),
			paymentOp("4", accountAddress, "GRECEIVER", "50", day(time.February, 5)),
			paymentOp("3", accountAddress, "GRECEIVER", "40", day(time.January, 20)),
			paymentOp("2", "GSOMEONE", accountAddress, "100", day(time.January, 15)),
			paymentOp("1", "GSOMEONE", accountAddress, "999", time.Date(2025, time.December, 20, 12, 0, 0, 0, time.UTC)),
		), nil)

		result, err := NewReportsService(horizonClient, distSvc, models).GetStatement(ctx, &stellarAccount, "wallet-1", "XLM", fromDate, toDate)
		require.NoError(t, err)
		summary := result.Summary.Assets[0]
		// Ending = today 1000 - later credit 300 + later debit 50; beginning = ending - 100 credited + 40 debited.
		assert.Equal(t, "750.0000000", summary.EndingBalance)
		assert.Equal(t, "690.0000000", summary.BeginningBalance)
		assert.Equal(t, "100.0000000", summary.TotalCredits)
		assert.Equal(t, "40.0000000", summary.TotalDebits)
		assert.True(t, summary.Reconciled)
		require.Len(t, summary.Transactions, 2, "only the period's payments are listed")
		assert.Equal(t, []string{"credit", "debit"}, []string{summary.Transactions[0].Type, summary.Transactions[1].Type})
	})

	t.Run("hitting the page cap inside the period refuses the statement as too long", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		distSvc.On("GetBalance", ctx, &stellarAccount, xlmAsset).Return(decimal.RequireFromString("1000000"), nil).Once()
		fullPage := make([]operations.Operation, 0, StatementPaymentsPageLimit)
		for i := 0; i < StatementPaymentsPageLimit; i++ {
			fullPage = append(fullPage, creditOp(fmt.Sprintf("op-%d", i), "1"))
		}
		horizonClient.On("Payments", mock.AnythingOfType("horizonclient.OperationRequest")).Return(pageOf(fullPage...), nil).Times(MaxStatementPaymentsPages)

		_, err := NewReportsService(horizonClient, distSvc, models).GetStatement(ctx, &stellarAccount, "wallet-1", "XLM", fromDate, toDate)
		assert.ErrorIs(t, err, ErrStatementPeriodTooLong)
	})

	t.Run("hitting the page cap before reaching the period refuses the statement as too old", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		distSvc.On("GetBalance", ctx, &stellarAccount, xlmAsset).Return(decimal.RequireFromString("1000000"), nil).Once()
		later := make([]operations.Operation, 0, StatementPaymentsPageLimit)
		for i := 0; i < StatementPaymentsPageLimit; i++ {
			later = append(later, paymentOp(fmt.Sprintf("op-%d", i), "GSOMEONE", accountAddress, "1", time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)))
		}
		horizonClient.On("Payments", mock.AnythingOfType("horizonclient.OperationRequest")).Return(pageOf(later...), nil).Times(MaxStatementPaymentsPages)

		_, err := NewReportsService(horizonClient, distSvc, &data.Models{}).GetStatement(ctx, &stellarAccount, "wallet-1", "XLM", fromDate, toDate)
		assert.ErrorIs(t, err, ErrStatementPeriodTooOld, "payments after the period are only summed, so the empty models are never touched")
	})

	t.Run("one scan covers every asset", func(t *testing.T) {
		usdc := data.Asset{Code: "USDC", Issuer: "GUSDCISSUER"}
		horizonClient := &horizonclient.MockClient{}
		defer horizonClient.AssertExpectations(t)
		distSvc := mocks.NewMockDistributionAccountService(t)
		distSvc.On("GetBalances", ctx, &stellarAccount).Return(map[data.Asset]decimal.Decimal{xlmAsset: decimal.Zero, usdc: decimal.Zero}, nil).Once()
		distSvc.On("GetBalance", ctx, &stellarAccount, xlmAsset).Return(decimal.RequireFromString("10"), nil).Once()
		distSvc.On("GetBalance", ctx, &stellarAccount, usdc).Return(decimal.RequireFromString("20"), nil).Once()
		usdcCredit := paymentOp("2", "GSOMEONE", accountAddress, "5", time.Date(2026, 1, 16, 12, 0, 0, 0, time.UTC))
		usdcCredit.Asset = base.Asset{Type: "credit_alphanum4", Code: "USDC", Issuer: "GUSDCISSUER"}
		horizonClient.On("Payments", mock.AnythingOfType("horizonclient.OperationRequest")).Return(pageOf(usdcCredit, creditOp("1", "3")), nil).Once()

		result, err := NewReportsService(horizonClient, distSvc, models).GetStatement(ctx, &stellarAccount, "wallet-1", "", fromDate, toDate)
		require.NoError(t, err)
		credits := map[string]string{}
		for _, a := range result.Summary.Assets {
			credits[a.Code] = a.TotalCredits
		}
		assert.Equal(t, map[string]string{"XLM": "3.0000000", "USDC": "5.0000000"}, credits)
	})
}

func TestReportsService_resolveAsset(t *testing.T) {
	ctx := context.Background()
	horizonClient := &horizonclient.MockClient{}
	distSvc := mocks.NewMockDistributionAccountService(t)
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	service := NewReportsService(horizonClient, distSvc, models)

	t.Run("resolves XLM alias", func(t *testing.T) {
		asset, err := service.resolveAsset(ctx, assets.XLMAssetCodeAlias)

		require.NoError(t, err)
		assert.Equal(t, assets.XLMAssetCode, asset.Code)
		assert.Empty(t, asset.Issuer)
	})

	t.Run("resolves XLM code", func(t *testing.T) {
		asset, err := service.resolveAsset(ctx, "XLM")

		require.NoError(t, err)
		assert.Equal(t, assets.XLMAssetCode, asset.Code)
		assert.Empty(t, asset.Issuer)
	})

	t.Run("resolves non-native asset from database", func(t *testing.T) {
		// Create a test asset in the database
		_ = data.CreateAssetFixture(t, ctx, dbPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335XZKGNW3PJ7RYF3KSP2M7QJ5V")

		asset, err := service.resolveAsset(ctx, "USDC")

		require.NoError(t, err)
		assert.Equal(t, "USDC", asset.Code)
		// The issuer might be different if there are multiple USDC assets, so just check code matches
		assert.NotEmpty(t, asset.Issuer)
	})

	t.Run("returns error for non-existent asset", func(t *testing.T) {
		asset, err := service.resolveAsset(ctx, "NONEXISTENT")

		require.Error(t, err)
		assert.Nil(t, asset)
		assert.Equal(t, ErrStatementAssetNotFound, err)
	})
}

func TestReportsService_counterpartyLabel(t *testing.T) {
	ctx := context.Background()
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)
	service := NewReportsService(&horizonclient.MockClient{}, mocks.NewMockDistributionAccountService(t), models)

	receiver := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{ExternalID: "test-receiver-123"})
	wallet := data.CreateWalletFixture(t, ctx, dbPool, "Test Wallet", "https://test.com", "test.com", "test://")
	receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)
	known := keypair.MustRandom().Address()
	_, err = dbPool.ExecContext(ctx, `UPDATE receiver_wallets SET stellar_address = $1 WHERE id = $2`, known, receiverWallet.ID)
	require.NoError(t, err)

	t.Run("labels a receiver's address and remembers the answer", func(t *testing.T) {
		labels := map[string]string{}
		label, err := service.counterpartyLabel(ctx, known, labels)
		require.NoError(t, err)
		assert.Equal(t, "test-receiver-123", label)
		assert.Equal(t, map[string]string{known: "test-receiver-123"}, labels)
	})

	t.Run("an unknown address has no label, and that is remembered too", func(t *testing.T) {
		unknown := keypair.MustRandom().Address()
		labels := map[string]string{}
		label, err := service.counterpartyLabel(ctx, unknown, labels)
		require.NoError(t, err)
		assert.Empty(t, label)
		assert.Contains(t, labels, unknown)
	})

	t.Run("a remembered address is not looked up again", func(t *testing.T) {
		noDB := NewReportsService(&horizonclient.MockClient{}, mocks.NewMockDistributionAccountService(t), &data.Models{})
		label, err := noDB.counterpartyLabel(ctx, known, map[string]string{known: "cached"})
		require.NoError(t, err)
		assert.Equal(t, "cached", label)
	})

	t.Run("database failures fail the statement instead of dropping labels", func(t *testing.T) {
		failing, cancel := context.WithCancel(ctx)
		cancel()
		_, err := service.counterpartyLabel(failing, keypair.MustRandom().Address(), map[string]string{})
		require.Error(t, err)

		op := operations.Payment{From: "GSOMEONE", To: known, Amount: "1", Asset: base.Asset{Type: "native"}}
		op.Base.TransactionHash = "tx-1"
		_, err = service.statementLine(failing, op, statementPayment{from: "GSOMEONE", to: known, amountStr: "1"}, "wallet-1", map[string]string{})
		require.Error(t, err)
	})
}

func TestFormatStellarAmount(t *testing.T) {
	tests := []struct {
		name     string
		input    decimal.Decimal
		expected string
	}{
		{
			name:     "zero",
			input:    decimal.Zero,
			expected: "0.0000000",
		},
		{
			name:     "small amount",
			input:    decimal.RequireFromString("0.0000001"),
			expected: "0.0000001",
		},
		{
			name:     "large amount",
			input:    decimal.RequireFromString("1000000.1234567"),
			expected: "1000000.1234567",
		},
		{
			name:     "negative amount",
			input:    decimal.RequireFromString("-100.5"),
			expected: "-100.5000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStellarAmount(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAssetMatchesHorizonAsset(t *testing.T) {
	tests := []struct {
		name     string
		asset    *data.Asset
		horizon  base.Asset
		expected bool
	}{
		{
			name:     "native matches native",
			asset:    &data.Asset{Code: assets.XLMAssetCode, Issuer: ""},
			horizon:  base.Asset{Type: "native"},
			expected: true,
		},
		{
			name:     "non-native matches with same code and issuer",
			asset:    &data.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335XZKGNW3PJ7RYF3KSP2M7QJ5V"},
			horizon:  base.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335XZKGNW3PJ7RYF3KSP2M7QJ5V"},
			expected: true,
		},
		{
			name:     "non-native does not match when only asset issuer is empty",
			asset:    &data.Asset{Code: "USDC", Issuer: ""},
			horizon:  base.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335XZKGNW3PJ7RYF3KSP2M7QJ5V"},
			expected: false, // Logic: asset.Issuer == h.Issuer OR (both empty) - neither condition met
		},
		{
			name:     "non-native does not match when only horizon issuer is empty",
			asset:    &data.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335XZKGNW3PJ7RYF3KSP2M7QJ5V"},
			horizon:  base.Asset{Code: "USDC", Issuer: ""},
			expected: false, // Logic: asset.Issuer == h.Issuer OR (both empty) - neither condition met
		},
		{
			name:     "non-native matches when both issuers are empty",
			asset:    &data.Asset{Code: "USDC", Issuer: ""},
			horizon:  base.Asset{Code: "USDC", Issuer: ""},
			expected: true, // Logic: both empty matches
		},
		{
			name:     "different codes",
			asset:    &data.Asset{Code: "USDC", Issuer: ""},
			horizon:  base.Asset{Code: "EURC", Issuer: ""},
			expected: false,
		},
		{
			name:     "different issuers",
			asset:    &data.Asset{Code: "USDC", Issuer: "ISSUER1"},
			horizon:  base.Asset{Code: "USDC", Issuer: "ISSUER2"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assetMatchesHorizonAsset(tt.asset, tt.horizon)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPaymentOperation(t *testing.T) {
	payment := operations.Payment{From: "GSOURCE", To: "GDEST", Amount: "100.0000000", Asset: base.Asset{Type: "native"}}
	payment.Base.ID = "12345"
	usdc := base.Asset{Code: "USDC", Issuer: "ISSUER"}
	pathPayment := operations.PathPayment{Payment: operations.Payment{From: "GSOURCE", To: "GDEST", Amount: "50.0000000", Asset: usdc}}
	pathPayment.Base.ID = "67890"
	strictSend := operations.PathPaymentStrictSend{Payment: pathPayment.Payment}

	testCases := []struct {
		name           string
		op             operations.Operation
		expectedAmount string
		expectedAsset  base.Asset
		expectedOpID   string
		expectedOK     bool
	}{
		{"payment", payment, "100.0000000", base.Asset{Type: "native"}, "12345", true},
		{"path payment", pathPayment, "50.0000000", usdc, "67890", true},
		{"path payment strict send", strictSend, "50.0000000", usdc, "67890", true},
		{"pointer to a payment", &payment, "100.0000000", base.Asset{Type: "native"}, "12345", true},
		{"not a payment", operations.CreateAccount{Account: "GACCOUNT"}, "", base.Asset{}, "", false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			from, to, amount, asset, opID, ok := extractPaymentOperation(tc.op)
			assert.Equal(t, tc.expectedOK, ok)
			if !tc.expectedOK {
				return
			}
			assert.Equal(t, "GSOURCE", from)
			assert.Equal(t, "GDEST", to)
			assert.Equal(t, tc.expectedAmount, amount)
			assert.Equal(t, tc.expectedAsset, asset)
			assert.Equal(t, tc.expectedOpID, opID)
		})
	}
}

func TestReportsService_GetStatement_ErrorCases(t *testing.T) {
	ctx := context.Background()
	accountAddress := keypair.MustRandom().Address()
	stellarAccount := schema.NewStellarEnvTransactionAccount(accountAddress)
	fromDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	toDate := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	t.Run("returns error when GetBalances fails", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		dbPool := testutils.GetDBConnectionPool(t)
		models, err := data.NewModels(dbPool)
		require.NoError(t, err)

		distSvc.On("GetBalances", ctx, &stellarAccount).
			Return(nil, errors.New("horizon error")).Once()

		service := NewReportsService(horizonClient, distSvc, models)

		result, err := service.GetStatement(ctx, &stellarAccount, "wallet-1", "", fromDate, toDate)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "getting balances")
	})

	t.Run("skips asset when GetBalance returns ErrNoBalanceForAsset", func(t *testing.T) {
		horizonClient := &horizonclient.MockClient{}
		distSvc := mocks.NewMockDistributionAccountService(t)
		dbPool := testutils.GetDBConnectionPool(t)
		models, err := data.NewModels(dbPool)
		require.NoError(t, err)

		xlmAsset := data.Asset{Code: assets.XLMAssetCode, Issuer: ""}

		distSvc.On("GetBalances", ctx, &stellarAccount).
			Return(map[data.Asset]decimal.Decimal{
				xlmAsset: decimal.Zero,
			}, nil).Once()

		distSvc.On("GetBalance", ctx, &stellarAccount, xlmAsset).
			Return(decimal.Zero, ErrNoBalanceForAsset).Once()

		service := NewReportsService(horizonClient, distSvc, models)

		result, err := service.GetStatement(ctx, &stellarAccount, "wallet-1", "", fromDate, toDate)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Summary.Assets)
	})
}
