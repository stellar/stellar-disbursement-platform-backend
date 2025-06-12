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
	dbConnectionPool := getConnectionPool(t)

	ctx := context.Background()
	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns only enabled wallets", func(t *testing.T) {
		wallets := ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[0].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallets[1].ID)

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterEnabledWallets, true))
		require.NoError(t, err)

		require.Len(t, actual, 1)
		require.Equal(t, wallets[1].ID, actual[0].ID)
	})

	t.Run("returns only disabled wallets", func(t *testing.T) {
		wallets := ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[0].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallets[1].ID)

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterEnabledWallets, false))
		require.NoError(t, err)

		require.Len(t, actual, 1)
		require.Equal(t, wallets[0].ID, actual[0].ID)
	})

	t.Run("returns user_managed wallet", func(t *testing.T) {
		wallets := ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterUserManaged, true))
		require.NoError(t, err)

		require.Len(t, actual, 1)
		require.Equal(t, wallets[0].ID, actual[0].ID)
	})

	t.Run("returns user_managed and enabled wallet", func(t *testing.T) {
		wallets := ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)

		MakeWalletUserManaged(t, ctx, dbConnectionPool, wallets[0].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, true, wallets[0].ID)
		EnableOrDisableWalletFixtures(t, ctx, dbConnectionPool, false, wallets[1].ID)

		actual, err := walletModel.FindWallets(ctx, NewFilter(FilterUserManaged, true), NewFilter(FilterEnabledWallets, true))
		require.NoError(t, err)

		require.Len(t, actual, 1)
		require.Equal(t, wallets[0].ID, actual[0].ID)
	})

	t.Run("returns empty array when no wallets", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		actual, err := walletModel.FindWallets(ctx)
		require.NoError(t, err)

		require.Equal(t, []Wallet{}, actual)
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
		deep_link_schema := "test-wallet://sdp"
		sep_10_client_domain := "www.test_wallet.com"
		assets := []string{xlm.ID, usdc.ID}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep_10_client_domain,
			DeepLinkSchema:    deep_link_schema,
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
		deep_link_schema := "test-wallet://sdp"
		sep_10_client_domain := "www.test_wallet.com"
		assets := []string{xlm.ID, xlm.ID, usdc.ID, usdc.ID}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep_10_client_domain,
			DeepLinkSchema:    deep_link_schema,
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
		deep_link_schema := "test-wallet://sdp"
		sep_10_client_domain := "www.test_wallet.com"
		assets := []string{xlm.ID, usdc.ID}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep_10_client_domain,
			DeepLinkSchema:    deep_link_schema,
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
			SEP10ClientDomain: sep_10_client_domain,
			DeepLinkSchema:    deep_link_schema,
			AssetsIDs:         assets,
		})
		assert.ErrorIs(t, err, ErrWalletNameAlreadyExists)
		assert.Nil(t, wallet)

		// Homepage error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          homepage,
			SEP10ClientDomain: sep_10_client_domain,
			DeepLinkSchema:    deep_link_schema,
			AssetsIDs:         assets,
		})
		assert.ErrorIs(t, err, ErrWalletHomepageAlreadyExists)
		assert.Nil(t, wallet)

		// Deep Link Schema error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          "https://another-wallet.com",
			DeepLinkSchema:    deep_link_schema,
			SEP10ClientDomain: sep_10_client_domain,
			AssetsIDs:         assets,
		})
		assert.ErrorIs(t, err, ErrWalletDeepLinkSchemaAlreadyExists)
		assert.Nil(t, wallet)

		// Deep Link Schema error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          "https://another-wallet.com",
			DeepLinkSchema:    deep_link_schema,
			SEP10ClientDomain: sep_10_client_domain,
			AssetsIDs:         assets,
		})
		assert.ErrorIs(t, err, ErrWalletDeepLinkSchemaAlreadyExists)
		assert.Nil(t, wallet)

		// Invalid Asset ID error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          "https://another-wallet.com",
			DeepLinkSchema:    "wallet://another-wallet/sdp",
			SEP10ClientDomain: sep_10_client_domain,
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
		deep_link_schema := "test-wallet://sdp"
		sep_10_client_domain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deep_link_schema, sep_10_client_domain)
		require.EqualError(t, err, "error getting or creating wallet: pq: duplicate key value violates unique constraint \"wallets_name_key\"")
		assert.Empty(t, wallet)
	})

	t.Run("inserts wallet successfully", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deep_link_schema := "test-wallet://sdp"
		sep_10_client_domain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deep_link_schema, sep_10_client_domain)
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
		deep_link_schema := "test-wallet://sdp"
		sep_10_client_domain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deep_link_schema, sep_10_client_domain)
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

func Test_WalletModelSoftDelete(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("soft deletes a wallet successfully", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)[0]

		assert.Nil(t, wallet.DeletedAt)

		wallet, err = walletModel.SoftDelete(ctx, wallet.ID)
		require.NoError(t, err)

		assert.NotNil(t, wallet.DeletedAt)
	})

	t.Run("doesn't delete an already deleted wallet", func(t *testing.T) {
		wallet := &ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool)[0]

		assert.Nil(t, wallet.DeletedAt)

		wallet, err = walletModel.SoftDelete(ctx, wallet.ID)
		require.NoError(t, err)

		assert.NotNil(t, wallet.DeletedAt)

		wallet, err = walletModel.SoftDelete(ctx, wallet.ID)
		assert.EqualError(t, err, ErrRecordNotFound.Error())
		assert.Nil(t, wallet)
	})

	t.Run("returns error when wallet doesn't exists", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		wallet, err := walletModel.SoftDelete(ctx, "unknown")
		assert.EqualError(t, err, ErrRecordNotFound.Error())
		assert.Nil(t, wallet)
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
