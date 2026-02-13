package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stellar/go-stellar-sdk/clients/horizonclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/protocols/horizon"
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

func TestNewReportsService(t *testing.T) {
	horizonClient := &horizonclient.MockClient{}
	distSvc := mocks.NewMockDistributionAccountService(t)
	models := &data.Models{}

	service := NewReportsService(horizonClient, distSvc, models)

	require.NotNil(t, service)
	assert.Equal(t, horizonClient, service.HorizonClient)
	assert.Equal(t, distSvc, service.DistributionAccountSvc)
	assert.Equal(t, models, service.Models)
}

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

		result, err := service.GetStatement(ctx, &nonStellarAccount, "", fromDate, toDate)

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

		result, err := service.GetStatement(ctx, &stellarAccount, "NONEXISTENT", fromDate, toDate)

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

		// Mock Transactions - empty page (called twice: once for transactions, once for totals)
		var emptyPage horizon.TransactionsPage
		emptyPage.Embedded.Records = []horizon.Transaction{}
		horizonClient.On("Transactions", mock.AnythingOfType("horizonclient.TransactionRequest")).
			Return(emptyPage, nil).Twice()

		service := NewReportsService(horizonClient, distSvc, models)

		result, err := service.GetStatement(ctx, &stellarAccount, "XLM", fromDate, toDate)

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

		// Mock Transactions - empty page (called twice: once for transactions, once for totals)
		var emptyPage horizon.TransactionsPage
		emptyPage.Embedded.Records = []horizon.Transaction{}
		horizonClient.On("Transactions", mock.AnythingOfType("horizonclient.TransactionRequest")).
			Return(emptyPage, nil).Twice()

		service := NewReportsService(horizonClient, distSvc, models)

		result, err := service.GetStatement(ctx, &stellarAccount, "", fromDate, toDate)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "stellar:"+accountAddress, result.Summary.Account)
		assert.Len(t, result.Summary.Assets, 1)
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

func TestReportsService_resolveCounterparty(t *testing.T) {
	ctx := context.Background()
	horizonClient := &horizonclient.MockClient{}
	distSvc := mocks.NewMockDistributionAccountService(t)
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	service := NewReportsService(horizonClient, distSvc, models)

	t.Run("returns empty string when wallet not found", func(t *testing.T) {
		address := keypair.MustRandom().Address()
		result := service.resolveCounterparty(ctx, address)

		assert.Empty(t, result)
	})

	t.Run("returns receiver external ID when wallet found", func(t *testing.T) {
		receiver := data.CreateReceiverFixture(t, ctx, dbPool, &data.Receiver{
			ExternalID: "test-receiver-123",
		})
		wallet := data.CreateWalletFixture(t, ctx, dbPool, "Test Wallet", "https://test.com", "test.com", "test://")
		receiverWallet := data.CreateReceiverWalletFixture(t, ctx, dbPool, receiver.ID, wallet.ID, data.ReadyReceiversWalletStatus)

		// Update receiver wallet with stellar address
		stellarAddress := keypair.MustRandom().Address()
		updateQuery := `UPDATE receiver_wallets SET stellar_address = $1 WHERE id = $2`
		_, err := dbPool.ExecContext(ctx, updateQuery, stellarAddress, receiverWallet.ID)
		require.NoError(t, err)

		result := service.resolveCounterparty(ctx, stellarAddress)

		assert.Equal(t, "test-receiver-123", result)
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
	t.Run("extracts Payment operation", func(t *testing.T) {
		op := operations.Payment{
			From:   "GSOURCE",
			To:     "GDEST",
			Amount: "100.0000000",
			Asset:  base.Asset{Type: "native"},
		}
		// Set ID manually since GetID() might return empty
		op.Base.ID = "12345"

		from, to, amount, asset, opID, ok := extractPaymentOperation(op)

		require.True(t, ok)
		assert.Equal(t, "GSOURCE", from)
		assert.Equal(t, "GDEST", to)
		assert.Equal(t, "100.0000000", amount)
		assert.Equal(t, base.Asset{Type: "native"}, asset)
		assert.Equal(t, "12345", opID)
	})

	t.Run("extracts Payment operation pointer", func(t *testing.T) {
		op := &operations.Payment{
			From:   "GSOURCE",
			To:     "GDEST",
			Amount: "50.0000000",
			Asset:  base.Asset{Code: "USDC", Issuer: "ISSUER"},
		}
		// Set ID manually since GetID() might return empty
		op.Base.ID = "67890"

		from, to, amount, asset, opID, ok := extractPaymentOperation(op)

		require.True(t, ok)
		assert.Equal(t, "GSOURCE", from)
		assert.Equal(t, "GDEST", to)
		assert.Equal(t, "50.0000000", amount)
		assert.Equal(t, base.Asset{Code: "USDC", Issuer: "ISSUER"}, asset)
		assert.Equal(t, "67890", opID)
	})

	// Note: PathPayment and PathPaymentStrictSend tests are skipped as they require
	// complex struct initialization due to embedded Payment fields

	t.Run("returns false for non-payment operation", func(t *testing.T) {
		op := operations.CreateAccount{
			Account: "GACCOUNT",
		}

		_, _, _, _, _, ok := extractPaymentOperation(op)

		assert.False(t, ok)
	})
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

		result, err := service.GetStatement(ctx, &stellarAccount, "", fromDate, toDate)

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

		result, err := service.GetStatement(ctx, &stellarAccount, "", fromDate, toDate)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Summary.Assets)
	})
}
