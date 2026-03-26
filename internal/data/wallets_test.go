package data

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_WalletColumnNamesWhenNested(t *testing.T) {
	testCases := []struct {
		tableReference string
		resultAlias    string
		includeDates   bool
		expected       string
	}{
		{
			tableReference: "",
			resultAlias:    "",
			includeDates:   false,
			expected: strings.Join([]string{
				"id",
				"name",
				"sep_10_client_domain",
				"homepage",
				"enabled",
				"deep_link_schema",
				"user_managed",
				"embedded",
			}, ",\n"),
		},
		{
			tableReference: "",
			resultAlias:    "wallet",
			includeDates:   false,
			expected: strings.Join([]string{
				`id AS "wallet.id"`,
				`name AS "wallet.name"`,
				`sep_10_client_domain AS "wallet.sep_10_client_domain"`,
				`homepage AS "wallet.homepage"`,
				`enabled AS "wallet.enabled"`,
				`deep_link_schema AS "wallet.deep_link_schema"`,
				`user_managed AS "wallet.user_managed"`,
				`embedded AS "wallet.embedded"`,
			}, ",\n"),
		},
		{
			tableReference: "w",
			resultAlias:    "",
			includeDates:   false,
			expected: strings.Join([]string{
				"w.id",
				"w.name",
				"w.sep_10_client_domain",
				"w.homepage",
				"w.enabled",
				"w.deep_link_schema",
				"w.user_managed",
				"w.embedded",
			}, ",\n"),
		},
		{
			tableReference: "w",
			resultAlias:    "wallet",
			includeDates:   true,
			expected: strings.Join([]string{
				`w.id AS "wallet.id"`,
				`w.name AS "wallet.name"`,
				`w.sep_10_client_domain AS "wallet.sep_10_client_domain"`,
				`w.homepage AS "wallet.homepage"`,
				`w.enabled AS "wallet.enabled"`,
				`w.deep_link_schema AS "wallet.deep_link_schema"`,
				`w.user_managed AS "wallet.user_managed"`,
				`w.embedded AS "wallet.embedded"`,
				`w.created_at AS "wallet.created_at"`,
				`w.updated_at AS "wallet.updated_at"`,
				`w.deleted_at AS "wallet.deleted_at"`,
			}, ",\n"),
		},
	}

	for _, tc := range testCases {
		t.Run(testCaseNameForScanText(t, tc.tableReference, tc.resultAlias), func(t *testing.T) {
			actual := WalletColumnNames(tc.tableReference, tc.resultAlias, tc.includeDates)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func Test_WalletModelGet(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when wallet is not found", func(t *testing.T) {
		_, err := walletModel.Get(ctx, "not-found")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns wallet successfully", func(t *testing.T) {
		expected := CreateWalletFixture(t, ctx, dbConnectionPool,
			"NewWallet",
			"https://newwallet.com",
			"newwallet.com",
			"newalletapp://")

		actual, err := walletModel.Get(ctx, expected.ID)
		require.NoError(t, err)

		assert.Equal(t, expected.ID, actual.ID)
		assert.Equal(t, expected.Name, actual.Name)
		assert.Equal(t, expected.DeepLinkSchema, actual.DeepLinkSchema)
		assert.Equal(t, expected.SEP10ClientDomain, actual.SEP10ClientDomain)
		assert.Empty(t, actual.Assets)
	})
}

func Test_WalletModelGetByWalletName(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when wallet is not found", func(t *testing.T) {
		_, err := walletModel.GetByWalletName(ctx, "invalid name")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns wallet successfully", func(t *testing.T) {
		expected := CreateWalletFixture(t, ctx, dbConnectionPool,
			"NewWallet",
			"https://newwallet.com",
			"newwallet.com",
			"newalletapp://")

		actual, err := walletModel.GetByWalletName(ctx, expected.Name)
		require.NoError(t, err)

		assert.Equal(t, expected.ID, actual.ID)
		assert.Equal(t, expected.Name, actual.Name)
		assert.Equal(t, expected.DeepLinkSchema, actual.DeepLinkSchema)
		assert.Equal(t, expected.SEP10ClientDomain, actual.SEP10ClientDomain)
		assert.Empty(t, actual.Assets)
	})
}

func Test_WalletModelGetAll(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	xlm := CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

	t.Run("returns all wallets successfully", func(t *testing.T) {
		wallets := ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		wallet1 := wallets[0]
		wallet2 := wallets[1]

		walletAssets1 := CreateWalletAssets(t, ctx, dbConnectionPool, wallet1.ID, []string{usdc.ID, xlm.ID})
		walletAssets2 := CreateWalletAssets(t, ctx, dbConnectionPool, wallet2.ID, []string{usdc.ID})

		actual, err := walletModel.GetAll(ctx)
		require.NoError(t, err)

		actualAssets1, err := walletModel.GetAssets(ctx, actual[0].ID)
		require.NoError(t, err)
		actualAssets2, err := walletModel.GetAssets(ctx, actual[1].ID)
		require.NoError(t, err)

		assert.Equal(t, wallet1.ID, actual[0].ID)
		assert.Equal(t, wallet1.Name, actual[0].Name)
		assert.Equal(t, wallet1.Homepage, actual[0].Homepage)
		assert.Equal(t, wallet1.DeepLinkSchema, actual[0].DeepLinkSchema)
		assert.Equal(t, wallet1.SEP10ClientDomain, actual[0].SEP10ClientDomain)
		assert.Len(t, actual[0].Assets, 2)
		assert.ElementsMatch(t, walletAssets1, actualAssets1)

		assert.Equal(t, wallet2.ID, actual[1].ID)
		assert.Equal(t, wallet2.Name, actual[1].Name)
		assert.Equal(t, wallet2.Homepage, actual[1].Homepage)
		assert.Equal(t, wallet2.DeepLinkSchema, actual[1].DeepLinkSchema)
		assert.Equal(t, wallet2.SEP10ClientDomain, actual[1].SEP10ClientDomain)
		assert.Len(t, actual[1].Assets, 1)
		assert.ElementsMatch(t, walletAssets2, actualAssets2)
	})

	t.Run("returns empty array when no wallets", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		actual, err := walletModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, []Wallet{}, actual)
	})
}

func Test_WalletModelFindWallets(t *testing.T) {
	models := SetupModels(t)
	dbConnectionPool := models.DBConnectionPool

	ctx := context.Background()
	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	usdc, outerErr := models.Assets.GetOrCreate(ctx, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")
	require.NoError(t, outerErr)
	xlm, outerErr := models.Assets.GetOrCreate(ctx, "XLM", "")
	require.NoError(t, outerErr)
	eurc, outerErr := models.Assets.GetOrCreate(ctx, "EURC", "GB3Q6QDZYTHWT7E5PVS3W7FUT5GVAFC5KSZFFLPU25GO7VTC3NM2ZTVO")
	require.NoError(t, outerErr)

	DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

	createTestWallets := func() []*Wallet {
		// Create wallets
		wallet0 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet0", "https://wallet0.com", "wallet0.com", "wallet0://")
		wallet1 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet1", "https://wallet1.com", "wallet1.com", "wallet1://")
		wallet2 := CreateWalletFixture(t, ctx, dbConnectionPool, "Wallet2", "https://wallet2.com", "wallet2.com", "wallet2://")

		// Associate assets with wallets
		// wallet0: USDC, XLM
		CreateWalletAssets(t, ctx, dbConnectionPool, wallet0.ID, []string{usdc.ID, xlm.ID})
		// wallet1: USDC, EURC
		CreateWalletAssets(t, ctx, dbConnectionPool, wallet1.ID, []string{usdc.ID, eurc.ID})
		// wallet2: XLM only
		CreateWalletAssets(t, ctx, dbConnectionPool, wallet2.ID, []string{xlm.ID})

		return []*Wallet{wallet0, wallet1, wallet2}
	}

	t.Run("returns only enabled wallets", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[0].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[1].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallets[2].ID)

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterEnabledWallets, true))
		require.NoError(t, err)

		require.Len(t, actual, 1)
		require.Equal(t, wallets[2].ID, actual[0].ID)
	})

	t.Run("returns only disabled wallets", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[0].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallets[1].ID)

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterEnabledWallets, false))
		require.NoError(t, err)

		require.Len(t, actual, 1)
		require.Equal(t, wallets[0].ID, actual[0].ID)
	})

	t.Run("returns user_managed wallet", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterUserManaged, true))
		require.NoError(t, err)

		require.Len(t, actual, 1)
		require.Equal(t, wallets[0].ID, actual[0].ID)
	})

	t.Run("filters embedded wallets", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		MakeWalletEmbedded(t, ctx, dbConnectionPool, wallets[1].ID)

		embeddedOnly, err := walletModel.FindWallets(ctx, NewFilter(FilterEmbedded, true))
		require.NoError(t, err)
		require.Len(t, embeddedOnly, 1)
		require.Equal(t, wallets[1].ID, embeddedOnly[0].ID)

		nonEmbedded, err := walletModel.FindWallets(ctx, NewFilter(FilterEmbedded, false))
		require.NoError(t, err)
		require.Len(t, nonEmbedded, 2)
		ids := []string{nonEmbedded[0].ID, nonEmbedded[1].ID}
		assert.NotContains(t, ids, wallets[1].ID)
	})

	t.Run("returns user_managed and enabled wallet", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallets[0].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[1].ID)

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterUserManaged, true), NewFilter(FilterEnabledWallets, true))
		require.NoError(t, err)

		require.Len(t, actual, 1)
		require.Equal(t, wallets[0].ID, actual[0].ID)
	})

	t.Run("returns empty array when no wallets", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })

		actual, err := walletModel.FindWallets(ctx)
		require.NoError(t, err)

		require.Equal(t, []Wallet{}, actual)
	})

	t.Run("returns wallets filtered by supported assets - single asset code", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Test filtering by single asset code
		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterSupportedAssets, []string{"USDC"}))
		require.NoError(t, err)

		assert.Len(t, actual, 2) // wallet1 and wallet2 support USDC
		walletIDs := []string{actual[0].ID, actual[1].ID}
		assert.Contains(t, walletIDs, wallets[0].ID)
		assert.Contains(t, walletIDs, wallets[1].ID)
	})

	t.Run("returns wallets filtered by supported assets - multiple asset codes", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Only wallet0 supports both USDC and XLM
		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterSupportedAssets, []string{"USDC", "XLM"}))
		require.NoError(t, err)

		assert.Len(t, actual, 1)
		assert.Equal(t, wallets[0].Name, actual[0].Name)
	})

	t.Run("returns wallets filtered by supported assets - asset ID", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		createTestWallets()

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterSupportedAssets, []string{usdc.ID}))
		require.NoError(t, err)

		assert.Len(t, actual, 2) // wallet0 and wallet1 support USDC
		for _, wallet := range actual {
			assert.Contains(t, []string{"Wallet0", "Wallet1"}, wallet.Name)
		}
	})

	t.Run("returns wallets filtered by supported assets - mixed codes and IDs", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		createTestWallets()

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterSupportedAssets, []string{usdc.ID, "XLM"}))
		require.NoError(t, err)

		assert.Len(t, actual, 1) // Only wallet0 supports both USDC (by ID) and XLM (by code)
		assert.Equal(t, "Wallet0", actual[0].Name)
	})

	t.Run("returns empty array when no wallets support specified assets", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		createTestWallets()

		bizant := CreateAssetFixture(t, ctx, dbConnectionPool, "BIZANT", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterSupportedAssets, []string{bizant.Code}))
		require.NoError(t, err)

		assert.Empty(t, actual)
	})

	t.Run("combines asset filtering with other filters", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Disable wallet2
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[1].ID)

		actual, err := walletModel.FindWallets(ctx,
			NewFilter(FilterSupportedAssets, []string{"USDC"}),
			NewFilter(FilterEnabledWallets, true))
		require.NoError(t, err)

		assert.Len(t, actual, 1) // Only wallet0 is enabled and supports USDC
		assert.Equal(t, "Wallet0", actual[0].Name)
	})

	t.Run("handles empty asset list in FilterSupportedAssets", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()
		// Empty asset list should be ignored and return all wallets
		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterSupportedAssets, []string{}))
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(actual), len(wallets))
	})

	t.Run("excludes deleted wallets by default", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Soft delete wallet0
		_, err := walletModel.SoftDelete(ctx, wallets[0].ID)
		require.NoError(t, err)

		// Should return only non-deleted wallets
		actual, err := walletModel.FindWallets(ctx)
		require.NoError(t, err)

		assert.Len(t, actual, 2)
		for _, wallet := range actual {
			assert.NotEqual(t, wallets[0].ID, wallet.ID)
		}
	})

	t.Run("includes deleted wallets when include_deleted=true", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Soft delete wallet0
		_, err := walletModel.SoftDelete(ctx, wallets[0].ID)
		require.NoError(t, err)

		// Should return all wallets including deleted
		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterIncludeDeleted, true))
		require.NoError(t, err)

		assert.Len(t, actual, 3)
		walletIDs := []string{actual[0].ID, actual[1].ID, actual[2].ID}
		assert.Contains(t, walletIDs, wallets[0].ID)
		assert.Contains(t, walletIDs, wallets[1].ID)
		assert.Contains(t, walletIDs, wallets[2].ID)
	})

	t.Run("excludes deleted wallets when include_deleted=false", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Soft delete wallet1
		_, err := walletModel.SoftDelete(ctx, wallets[1].ID)
		require.NoError(t, err)

		// Explicitly exclude deleted wallets
		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterIncludeDeleted, false))
		require.NoError(t, err)

		assert.Len(t, actual, 2)
		for _, wallet := range actual {
			assert.NotEqual(t, wallets[1].ID, wallet.ID)
		}
	})

	t.Run("combines deleted filter with other filters", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Soft delete wallet0
		_, err := walletModel.SoftDelete(ctx, wallets[0].ID)
		require.NoError(t, err)

		// Disable wallet2
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[2].ID)

		// Should return only enabled and non-deleted wallets
		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterEnabledWallets, true))
		require.NoError(t, err)

		assert.Len(t, actual, 1)
		assert.Equal(t, wallets[1].ID, actual[0].ID)
	})

	t.Run("combines include_deleted with enabled filter", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Soft delete wallet0
		_, err := walletModel.SoftDelete(ctx, wallets[0].ID)
		require.NoError(t, err)

		// Disable wallet1
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[1].ID)

		// Should return only enabled wallets including deleted ones
		actual, err := walletModel.FindWallets(ctx,
			NewFilter(FilterIncludeDeleted, true),
			NewFilter(FilterEnabledWallets, true))
		require.NoError(t, err)

		assert.Len(t, actual, 2) // wallet0 (deleted, enabled) and wallet2 (not deleted, enabled)
		walletIDs := []string{actual[0].ID, actual[1].ID}
		assert.Contains(t, walletIDs, wallets[0].ID)
		assert.Contains(t, walletIDs, wallets[2].ID)
	})

	t.Run("combines include_deleted with supported_assets filter", func(t *testing.T) {
		t.Cleanup(func() { DeleteAllWalletFixtures(t, ctx, dbConnectionPool) })
		wallets := createTestWallets()

		// Soft delete wallet0 (which supports USDC and XLM)
		_, err := walletModel.SoftDelete(ctx, wallets[0].ID)
		require.NoError(t, err)

		// Search for USDC wallets including deleted
		actual, err := walletModel.FindWallets(ctx,
			NewFilter(FilterIncludeDeleted, true),
			NewFilter(FilterSupportedAssets, []string{"USDC"}))
		require.NoError(t, err)

		assert.Len(t, actual, 2) // wallet0 (deleted) and wallet1 (not deleted) both support USDC
		walletIDs := []string{actual[0].ID, actual[1].ID}
		assert.Contains(t, walletIDs, wallets[0].ID)
		assert.Contains(t, walletIDs, wallets[1].ID)
	})
}

func Test_WalletModelInsert(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	xlm := CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")
	usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	t.Run("inserts wallet successfully", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deepLinkSchema := "test-wallet://sdp"
		sep10ClientDomain := "www.test_wallet.com"
		assets := []string{xlm.ID, usdc.ID}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep10ClientDomain,
			DeepLinkSchema:    deepLinkSchema,
			AssetsIDs:         assets,
		})
		require.NoError(t, err)
		assert.NotNil(t, wallet)

		insertedWallet, err := walletModel.Get(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Equal(t, insertedWallet.ID, wallet.ID)
		assert.Equal(t, insertedWallet.Homepage, wallet.Homepage)
		assert.Equal(t, insertedWallet.DeepLinkSchema, wallet.DeepLinkSchema)
		assert.Equal(t, insertedWallet.SEP10ClientDomain, wallet.SEP10ClientDomain)

		assetsDB, err := walletModel.GetAssets(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, assetsDB, 2)
		assert.ElementsMatch(t, []Asset{
			{
				ID:        usdc.ID,
				Code:      usdc.Code,
				Issuer:    usdc.Issuer,
				CreatedAt: usdc.CreatedAt,
				UpdatedAt: usdc.UpdatedAt,
				DeletedAt: usdc.DeletedAt,
			},
			{
				ID:        xlm.ID,
				Code:      xlm.Code,
				Issuer:    xlm.Issuer,
				CreatedAt: xlm.CreatedAt,
				UpdatedAt: xlm.UpdatedAt,
				DeletedAt: xlm.DeletedAt,
			},
		}, assetsDB)
	})

	// Ensure that only insert one of each entry
	t.Run("duplicated assets IDs", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deepLinkSchema := "test-wallet://sdp"
		sep10ClientDomain := "www.test_wallet.com"
		assets := []string{xlm.ID, xlm.ID, usdc.ID, usdc.ID}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep10ClientDomain,
			DeepLinkSchema:    deepLinkSchema,
			AssetsIDs:         assets,
		})
		require.NoError(t, err)
		assert.NotNil(t, wallet)

		insertedWallet, err := walletModel.Get(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Equal(t, insertedWallet.ID, wallet.ID)
		assert.Equal(t, insertedWallet.Homepage, wallet.Homepage)
		assert.Equal(t, insertedWallet.DeepLinkSchema, wallet.DeepLinkSchema)
		assert.Equal(t, insertedWallet.SEP10ClientDomain, wallet.SEP10ClientDomain)

		assetsDB, err := walletModel.GetAssets(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, assetsDB, 2)
		assert.ElementsMatch(t, []Asset{
			{
				ID:        usdc.ID,
				Code:      usdc.Code,
				Issuer:    usdc.Issuer,
				CreatedAt: usdc.CreatedAt,
				UpdatedAt: usdc.UpdatedAt,
				DeletedAt: usdc.DeletedAt,
			},
			{
				ID:        xlm.ID,
				Code:      xlm.Code,
				Issuer:    xlm.Issuer,
				CreatedAt: xlm.CreatedAt,
				UpdatedAt: xlm.UpdatedAt,
				DeletedAt: xlm.DeletedAt,
			},
		}, assetsDB)
	})

	t.Run("returns error when violates database constraints", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deepLinkSchema := "test-wallet://sdp"
		sep10ClientDomain := "www.test_wallet.com"
		assets := []string{xlm.ID, usdc.ID}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep10ClientDomain,
			DeepLinkSchema:    deepLinkSchema,
			AssetsIDs:         assets,
		})
		require.NoError(t, err)
		assert.NotNil(t, wallet)

		insertedWallet, err := walletModel.Get(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Equal(t, insertedWallet.ID, wallet.ID)
		assert.Equal(t, insertedWallet.Homepage, wallet.Homepage)
		assert.Equal(t, insertedWallet.DeepLinkSchema, wallet.DeepLinkSchema)
		assert.Equal(t, insertedWallet.SEP10ClientDomain, wallet.SEP10ClientDomain)

		// Name error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep10ClientDomain,
			DeepLinkSchema:    deepLinkSchema,
			AssetsIDs:         assets,
		})
		assert.ErrorIs(t, err, ErrWalletNameAlreadyExists)
		assert.Nil(t, wallet)

		// Homepage error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          homepage,
			SEP10ClientDomain: sep10ClientDomain,
			DeepLinkSchema:    deepLinkSchema,
			AssetsIDs:         assets,
		})
		assert.ErrorIs(t, err, ErrWalletHomepageAlreadyExists)
		assert.Nil(t, wallet)

		// Deep Link Schema error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          "https://another-wallet.com",
			DeepLinkSchema:    deepLinkSchema,
			SEP10ClientDomain: sep10ClientDomain,
			AssetsIDs:         assets,
		})
		assert.ErrorIs(t, err, ErrWalletDeepLinkSchemaAlreadyExists)
		assert.Nil(t, wallet)

		// Deep Link Schema error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          "https://another-wallet.com",
			DeepLinkSchema:    deepLinkSchema,
			SEP10ClientDomain: sep10ClientDomain,
			AssetsIDs:         assets,
		})
		assert.ErrorIs(t, err, ErrWalletDeepLinkSchemaAlreadyExists)
		assert.Nil(t, wallet)

		// Invalid Asset ID error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          "https://another-wallet.com",
			DeepLinkSchema:    "wallet://another-wallet/sdp",
			SEP10ClientDomain: sep10ClientDomain,
			AssetsIDs:         []string{"invalid-id"},
		})
		assert.ErrorIs(t, err, ErrInvalidAssetID)
		assert.Nil(t, wallet)
	})
}

func Test_WalletModelGetOrCreate(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error wallet name already been used", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		CreateWalletFixture(t, ctx, dbConnectionPool,
			"test_wallet",
			"https://www.new_wallet.com",
			"www.new_wallet.com",
			"new_wallet://")

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deepLinkSchema := "test-wallet://sdp"
		sep10ClientDomain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deepLinkSchema, sep10ClientDomain, false)
		require.ErrorContains(t, err, "error getting or creating wallet: pq: duplicate key value violates unique constraint \"wallets_name_key\"")
		assert.Empty(t, wallet)
	})

	t.Run("inserts wallet successfully", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deepLinkSchema := "test-wallet://sdp"
		sep10ClientDomain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deepLinkSchema, sep10ClientDomain, false)
		require.NoError(t, err)
		assert.Equal(t, "test_wallet", wallet.Name)
		assert.Equal(t, "https://www.test_wallet.com", wallet.Homepage)
		assert.Equal(t, "test-wallet://sdp", wallet.DeepLinkSchema)
		assert.Equal(t, "www.test_wallet.com", wallet.SEP10ClientDomain)
	})

	t.Run("returns wallet successfully", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		expected := CreateWalletFixture(t, ctx, dbConnectionPool,
			"test_wallet",
			"https://www.test_wallet.com",
			"www.test_wallet.com",
			"test-wallet://sdp")

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deepLinkSchema := "test-wallet://sdp"
		sep10ClientDomain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deepLinkSchema, sep10ClientDomain, false)
		require.NoError(t, err)
		assert.Equal(t, expected.ID, wallet.ID)
	})
}

func Test_WalletModelGetAssets(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	xlm := CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

	t.Run("return empty when wallet doesn't have assets", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool,
			"NewWallet",
			"https://newwallet.com",
			"newwallet.com",
			"newalletapp://")

		assets, err := walletModel.GetAssets(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Empty(t, assets)
	})

	t.Run("return wallet's assets", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool,
			"NewWallet",
			"https://newwallet.com",
			"newwallet.com",
			"newalletapp://")

		CreateWalletAssets(t, ctx, dbConnectionPool, wallet.ID, []string{usdc.ID, xlm.ID})

		assets, err := walletModel.GetAssets(ctx, wallet.ID)
		require.NoError(t, err)

		assert.ElementsMatch(t, []Asset{
			{
				ID:        usdc.ID,
				Code:      usdc.Code,
				Issuer:    usdc.Issuer,
				CreatedAt: usdc.CreatedAt,
				UpdatedAt: usdc.UpdatedAt,
				DeletedAt: usdc.DeletedAt,
			},
			{
				ID:        xlm.ID,
				Code:      xlm.Code,
				Issuer:    xlm.Issuer,
				CreatedAt: xlm.CreatedAt,
				UpdatedAt: xlm.UpdatedAt,
				DeletedAt: xlm.DeletedAt,
			},
		}, assets)
	})
}

func Test_WalletModelHasPendingReceiverWallets(t *testing.T) {
	models := SetupModels(t)
	ctx := context.Background()

	walletModel := models.Wallets

	t.Run("returns false when wallet has no receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]

		hasPending, err := walletModel.HasPendingReceiverWallets(ctx, wallet.ID)
		require.NoError(t, err)
		assert.False(t, hasPending)
	})

	t.Run("returns true when wallet has DRAFT receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]
		receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, nil)
		CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

		hasPending, err := walletModel.HasPendingReceiverWallets(ctx, wallet.ID)
		require.NoError(t, err)
		assert.True(t, hasPending)
	})

	t.Run("returns true when wallet has READY receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]
		receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, nil)
		CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

		hasPending, err := walletModel.HasPendingReceiverWallets(ctx, wallet.ID)
		require.NoError(t, err)
		assert.True(t, hasPending)
	})

	t.Run("returns false when wallet only has REGISTERED receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]
		receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, nil)
		CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		hasPending, err := walletModel.HasPendingReceiverWallets(ctx, wallet.ID)
		require.NoError(t, err)
		assert.False(t, hasPending)
	})

	t.Run("returns false when wallet only has FLAGGED receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]
		receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, nil)
		CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, FlaggedReceiversWalletStatus)

		hasPending, err := walletModel.HasPendingReceiverWallets(ctx, wallet.ID)
		require.NoError(t, err)
		assert.False(t, hasPending)
	})
}

func Test_WalletModelSoftDelete(t *testing.T) {
	models := SetupModels(t)
	ctx := context.Background()

	walletModel := models.Wallets

	t.Run("soft deletes a wallet successfully", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]

		assert.Nil(t, wallet.DeletedAt)

		deletedWallet, err := walletModel.SoftDelete(ctx, wallet.ID)
		require.NoError(t, err)

		assert.NotNil(t, deletedWallet.DeletedAt)
	})

	t.Run("doesn't delete an already deleted wallet", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]

		assert.Nil(t, wallet.DeletedAt)

		deletedWallet, err := walletModel.SoftDelete(ctx, wallet.ID)
		require.NoError(t, err)

		assert.NotNil(t, deletedWallet.DeletedAt)

		deletedWallet, err = walletModel.SoftDelete(ctx, wallet.ID)
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, deletedWallet)
	})

	t.Run("returns error when wallet doesn't exists", func(t *testing.T) {
		DeleteAllFixtures(t, ctx, models.DBConnectionPool)

		wallet, err := walletModel.SoftDelete(ctx, "unknown")
		assert.ErrorIs(t, err, ErrRecordNotFound)
		assert.Nil(t, wallet)
	})

	t.Run("returns error when wallet has DRAFT receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]
		receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, nil)
		CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, DraftReceiversWalletStatus)

		deletedWallet, err := walletModel.SoftDelete(ctx, wallet.ID)
		assert.ErrorIs(t, err, ErrWalletInUse)
		assert.Nil(t, deletedWallet)
	})

	t.Run("returns error when wallet has READY receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]
		receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, nil)
		CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, ReadyReceiversWalletStatus)

		deletedWallet, err := walletModel.SoftDelete(ctx, wallet.ID)
		assert.ErrorIs(t, err, ErrWalletInUse)
		assert.Nil(t, deletedWallet)
	})

	t.Run("allows deletion when wallet only has REGISTERED receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]
		receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, nil)
		CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, RegisteredReceiversWalletStatus)

		deletedWallet, err := walletModel.SoftDelete(ctx, wallet.ID)
		require.NoError(t, err)
		assert.NotNil(t, deletedWallet.DeletedAt)
	})

	t.Run("allows deletion when wallet only has FLAGGED receiver_wallets", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, models.DBConnectionPool)[0]
		receiver := CreateReceiverFixture(t, ctx, models.DBConnectionPool, nil)
		CreateReceiverWalletFixture(t, ctx, models.DBConnectionPool, receiver.ID, wallet.ID, FlaggedReceiversWalletStatus)

		deletedWallet, err := walletModel.SoftDelete(ctx, wallet.ID)
		require.NoError(t, err)
		assert.NotNil(t, deletedWallet.DeletedAt)
	})
}

func Test_WalletModelUpdate(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	ctx := context.Background()
	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}
	DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

	xlm := CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")
	usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")

	t.Run("updates all fields successfully", func(t *testing.T) {
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool,
			"Mechanicus Wallet",
			"https://mars.forge",
			"mars.forge",
			"omnissiah://")

		newName := "Cawl's Digital Vault"
		newHomepage := "https://cawl.mechanicus"
		newDeepLink := "archmagos://sdp"
		newSEP10 := "cawl.mechanicus"
		newEnabled := false

		update := WalletUpdate{
			Name:              &newName,
			Homepage:          &newHomepage,
			DeepLinkSchema:    &newDeepLink,
			SEP10ClientDomain: &newSEP10,
			Enabled:           &newEnabled,
			AssetsIDs:         &[]string{xlm.ID, usdc.ID},
		}

		updatedWallet, err := walletModel.Update(ctx, wallet.ID, update)
		require.NoError(t, err)

		assert.Equal(t, newName, updatedWallet.Name)
		assert.Equal(t, newHomepage, updatedWallet.Homepage)
		assert.Equal(t, newDeepLink, updatedWallet.DeepLinkSchema)
		assert.Equal(t, newSEP10, updatedWallet.SEP10ClientDomain)
		assert.Equal(t, newEnabled, updatedWallet.Enabled)
		assert.Len(t, updatedWallet.Assets, 2)
	})

	t.Run("returns error for non-existent wallet", func(t *testing.T) {
		b := true
		update := WalletUpdate{
			Enabled: &b,
		}

		_, err := walletModel.Update(ctx, "lost-in-the-warp", update)
		assert.ErrorContains(t, err, "record not found")
	})

	t.Run("returns error for duplicate name", func(t *testing.T) {
		inquisitorWallet := CreateWalletFixture(t, ctx, dbConnectionPool,
			"Inquisition Funds",
			"https://ordo.hereticus",
			"ordo.hereticus",
			"emperor://")

		astartesPurse := CreateWalletFixture(t, ctx, dbConnectionPool,
			"Astartes Treasury",
			"https://ultramar.savings",
			"ultramar.savings",
			"guilliman://")

		duplicateName := inquisitorWallet.Name
		update := WalletUpdate{
			Name: &duplicateName,
		}

		_, err := walletModel.Update(ctx, astartesPurse.ID, update)
		assert.ErrorIs(t, err, ErrWalletNameAlreadyExists)
	})
}

func Test_WalletModelUpdate_AssetHandling(t *testing.T) {
	dbConnectionPool := getConnectionPool(t)

	ctx := context.Background()
	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}
	DeleteAllAssetFixtures(t, ctx, dbConnectionPool)

	xlm := CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")
	usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5")
	bolt := CreateAssetFixture(t, ctx, dbConnectionPool, "BOLT", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	t.Run("replaces assets when assets field is provided", func(t *testing.T) {
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool,
			"Rogue Trader Vault",
			"https://von.ravensburg",
			"von.ravensburg",
			"tradervault://")

		CreateWalletAssets(t, ctx, dbConnectionPool, wallet.ID, []string{xlm.ID, usdc.ID})

		newAssets := []string{bolt.ID}
		update := WalletUpdate{
			AssetsIDs: &newAssets,
		}

		updatedWallet, err := walletModel.Update(ctx, wallet.ID, update)
		require.NoError(t, err)

		assert.Len(t, updatedWallet.Assets, 1)
		assert.Equal(t, bolt.ID, updatedWallet.Assets[0].ID)
	})

	t.Run("clears assets when empty array is provided", func(t *testing.T) {
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool,
			"Tau Commerce Node",
			"https://tau.empire",
			"tau.empire",
			"greatergood://")
		CreateWalletAssets(t, ctx, dbConnectionPool, wallet.ID, []string{xlm.ID, usdc.ID})

		emptyAssets := []string{}
		update := WalletUpdate{
			AssetsIDs: &emptyAssets,
		}

		updatedWallet, err := walletModel.Update(ctx, wallet.ID, update)
		require.NoError(t, err)

		assert.Len(t, updatedWallet.Assets, 0)
	})

	t.Run("preserves assets when assets field is not provided", func(t *testing.T) {
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool,
			"Ecclesiarchy Treasury",
			"https://terra.sanctum",
			"terra.sanctum",
			"imperator://")
		CreateWalletAssets(t, ctx, dbConnectionPool, wallet.ID, []string{xlm.ID, usdc.ID})

		newName := "High Lords' Reserve"
		update := WalletUpdate{
			Name: &newName,
		}

		updatedWallet, err := walletModel.Update(ctx, wallet.ID, update)
		require.NoError(t, err)

		assert.Equal(t, newName, updatedWallet.Name)
		assert.Len(t, updatedWallet.Assets, 2)
	})

	t.Run("returns error when no fields provided", func(t *testing.T) {
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool,
			"Silent King's Hoard",
			"https://necron.dynasties",
			"necron.dynasties",
			"szarekh://")

		update := WalletUpdate{}

		_, err := walletModel.Update(ctx, wallet.ID, update)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no fields provided for update")
	})
}
