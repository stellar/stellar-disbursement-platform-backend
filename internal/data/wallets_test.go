package data

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_WalletModelGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when wallet is not found", func(t *testing.T) {
		_, err := walletModel.Get(ctx, "not-found")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns wallet successfully", func(t *testing.T) {
		expected := CreateWalletFixture(t, ctx, dbConnectionPool.SqlxDB(),
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
		assert.Empty(t, actual.Countries)
		assert.Empty(t, actual.Assets)
	})
}

func Test_WalletModelGetByWalletName(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when wallet is not found", func(t *testing.T) {
		_, err := walletModel.GetByWalletName(ctx, "invalid name")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns wallet successfully", func(t *testing.T) {
		expected := CreateWalletFixture(t, ctx, dbConnectionPool.SqlxDB(),
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
		assert.Empty(t, actual.Countries)
		assert.Empty(t, actual.Assets)
	})
}

func Test_WalletModelGetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	xlm := CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")

	t.Run("returns all wallets successfully", func(t *testing.T) {
		wallets := ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool.SqlxDB())

		wallet1 := wallets[0]
		wallet2 := wallets[1]

		CreateWalletAssets(t, ctx, dbConnectionPool, wallet1.ID, []string{usdc.ID, xlm.ID})
		CreateWalletAssets(t, ctx, dbConnectionPool, wallet2.ID, []string{usdc.ID})

		CreateWalletCountries(t, ctx, dbConnectionPool, wallet1.ID, []string{"UKR", "COL", "USA"})
		CreateWalletCountries(t, ctx, dbConnectionPool, wallet2.ID, []string{"UKR", "BRA", "ARG", "MEX"})

		actual, err := walletModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, wallet1.ID, actual[0].ID)
		assert.Equal(t, wallet1.Name, actual[0].Name)
		assert.Equal(t, wallet1.Homepage, actual[0].Homepage)
		assert.Equal(t, wallet1.DeepLinkSchema, actual[0].DeepLinkSchema)
		assert.Equal(t, wallet1.SEP10ClientDomain, actual[0].SEP10ClientDomain)
		assert.Len(t, actual[0].Countries, 3)
		assert.Len(t, actual[0].Assets, 2)
		assert.ElementsMatch(t, WalletCountries{
			{
				Code: "COL",
				Name: "Colombia",
			},
			{
				Code: "UKR",
				Name: "Ukraine",
			},
			{
				Code: "USA",
				Name: "United States of America",
			},
		}, actual[0].Countries)
		assert.ElementsMatch(t, WalletAssets{
			{
				ID:     usdc.ID,
				Code:   usdc.Code,
				Issuer: usdc.Issuer,
			},
			{
				ID:     xlm.ID,
				Code:   xlm.Code,
				Issuer: xlm.Issuer,
			},
		}, actual[0].Assets)

		assert.Equal(t, wallet2.ID, actual[1].ID)
		assert.Equal(t, wallet2.Name, actual[1].Name)
		assert.Equal(t, wallet2.Homepage, actual[1].Homepage)
		assert.Equal(t, wallet2.DeepLinkSchema, actual[1].DeepLinkSchema)
		assert.Equal(t, wallet2.SEP10ClientDomain, actual[1].SEP10ClientDomain)
		// assert.ElementsMatch(t, WalletCountries{
		// 	{
		// 		Code: "UKR",
		// 		Name: "Ukraine",
		// 	},
		// 	{
		// 		Code: "BRA",
		// 		Name: "Brazil",
		// 	},
		// 	{
		// 		Code: "ARG",
		// 		Name: "Argentina",
		// 	},
		// 	{
		// 		Code: "MEX",
		// 		Name: "Mexico",
		// 	},
		// }, actual[1].Countries)
		assert.ElementsMatch(t, WalletAssets{
			{
				ID:     usdc.ID,
				Code:   usdc.Code,
				Issuer: usdc.Issuer,
			},
		}, actual[1].Assets)
	})

	t.Run("returns empty array when no wallets", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool.SqlxDB())
		actual, err := walletModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, []Wallet{}, actual)
	})
}

func Test_WalletModelInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	DeleteAllAssetFixtures(t, ctx, dbConnectionPool)
	xlm := CreateAssetFixture(t, ctx, dbConnectionPool, "XLM", "")
	usdc := CreateAssetFixture(t, ctx, dbConnectionPool, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")

	t.Run("inserts wallet successfully", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deep_link_schema := "test_wallet://"
		sep_10_client_domain := "www.test_wallet.com"
		assets := []string{xlm.ID, usdc.ID}
		countries := []string{"UKR", "USA", "BRA"}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep_10_client_domain,
			DeepLinkSchema:    deep_link_schema,
			AssetsIDs:         assets,
			CountriesCodes:    countries,
		})
		require.NoError(t, err)
		assert.NotNil(t, wallet)

		insertedWallet, err := walletModel.Get(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Equal(t, insertedWallet.ID, wallet.ID)
		assert.Equal(t, insertedWallet.Homepage, wallet.Homepage)
		assert.Equal(t, insertedWallet.DeepLinkSchema, wallet.DeepLinkSchema)
		assert.Equal(t, insertedWallet.SEP10ClientDomain, wallet.SEP10ClientDomain)

		countriesDB, err := walletModel.GetCountries(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, countriesDB, 3)
		assert.ElementsMatch(t, []Country{
			{
				Code: "BRA",
				Name: "Brazil",
			},
			{
				Code: "UKR",
				Name: "Ukraine",
			},
			{
				Code: "USA",
				Name: "United States of America",
			},
		}, countriesDB)

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
	t.Run("duplicated countries codes and assets IDs", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deep_link_schema := "test_wallet://"
		sep_10_client_domain := "www.test_wallet.com"
		assets := []string{xlm.ID, xlm.ID, usdc.ID, usdc.ID}
		countries := []string{"UKR", "UKR"}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep_10_client_domain,
			DeepLinkSchema:    deep_link_schema,
			AssetsIDs:         assets,
			CountriesCodes:    countries,
		})
		require.NoError(t, err)
		assert.NotNil(t, wallet)

		insertedWallet, err := walletModel.Get(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Equal(t, insertedWallet.ID, wallet.ID)
		assert.Equal(t, insertedWallet.Homepage, wallet.Homepage)
		assert.Equal(t, insertedWallet.DeepLinkSchema, wallet.DeepLinkSchema)
		assert.Equal(t, insertedWallet.SEP10ClientDomain, wallet.SEP10ClientDomain)

		countriesDB, err := walletModel.GetCountries(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Len(t, countriesDB, 1)
		assert.ElementsMatch(t, []Country{
			{
				Code: "UKR",
				Name: "Ukraine",
			},
		}, countriesDB)

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
		deep_link_schema := "test_wallet://"
		sep_10_client_domain := "www.test_wallet.com"
		assets := []string{xlm.ID, usdc.ID}
		countries := []string{"UKR", "USA", "BRA"}

		wallet, err := walletModel.Insert(ctx, WalletInsert{
			Name:              name,
			Homepage:          homepage,
			SEP10ClientDomain: sep_10_client_domain,
			DeepLinkSchema:    deep_link_schema,
			AssetsIDs:         assets,
			CountriesCodes:    countries,
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
			CountriesCodes:    countries,
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
			CountriesCodes:    countries,
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
			CountriesCodes:    countries,
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
			CountriesCodes:    countries,
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
			CountriesCodes:    countries,
		})
		assert.ErrorIs(t, err, ErrInvalidAssetID)
		assert.Nil(t, wallet)

		// Invalid Country Code error
		wallet, err = walletModel.Insert(ctx, WalletInsert{
			Name:              "Another Wallet",
			Homepage:          "https://another-wallet.com",
			DeepLinkSchema:    "wallet://another-wallet/sdp",
			SEP10ClientDomain: sep_10_client_domain,
			AssetsIDs:         assets,
			CountriesCodes:    []string{"AAA"},
		})
		assert.ErrorIs(t, err, ErrInvalidCountryCode)
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
		CreateWalletFixture(t, ctx, dbConnectionPool.SqlxDB(),
			"test_wallet",
			"https://www.new_wallet.com",
			"www.new_wallet.com",
			"new_wallet://")

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deep_link_schema := "test_wallet://"
		sep_10_client_domain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deep_link_schema, sep_10_client_domain)
		require.EqualError(t, err, "error getting or creating wallet: pq: duplicate key value violates unique constraint \"wallets_name_key\"")
		assert.Empty(t, wallet)
	})

	t.Run("inserts wallet successfully", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deep_link_schema := "test_wallet://"
		sep_10_client_domain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deep_link_schema, sep_10_client_domain)
		require.NoError(t, err)
		assert.Equal(t, "test_wallet", wallet.Name)
		assert.Equal(t, "https://www.test_wallet.com", wallet.Homepage)
		assert.Equal(t, "test_wallet://", wallet.DeepLinkSchema)
		assert.Equal(t, "www.test_wallet.com", wallet.SEP10ClientDomain)
	})

	t.Run("returns wallet successfully", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		expected := CreateWalletFixture(t, ctx, dbConnectionPool.SqlxDB(),
			"test_wallet",
			"https://www.test_wallet.com",
			"www.test_wallet.com",
			"test_wallet://")

		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deep_link_schema := "test_wallet://"
		sep_10_client_domain := "www.test_wallet.com"

		wallet, err := walletModel.GetOrCreate(ctx, name, homepage, deep_link_schema, sep_10_client_domain)
		require.NoError(t, err)
		assert.Equal(t, expected.ID, wallet.ID)
	})
}

func Test_WalletModelGetCountries(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	walletModel := &WalletModel{dbConnectionPool: dbConnectionPool}

	t.Run("return empty when wallet doesn't have countries", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool.SqlxDB(),
			"NewWallet",
			"https://newwallet.com",
			"newwallet.com",
			"newalletapp://")

		countries, err := walletModel.GetCountries(ctx, wallet.ID)
		require.NoError(t, err)
		assert.Empty(t, countries)
	})

	t.Run("return wallet's countries", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool.SqlxDB(),
			"NewWallet",
			"https://newwallet.com",
			"newwallet.com",
			"newalletapp://")

		CreateWalletCountries(t, ctx, dbConnectionPool, wallet.ID, []string{"UKR", "USA"})

		countries, err := walletModel.GetCountries(ctx, wallet.ID)
		require.NoError(t, err)

		assert.ElementsMatch(t, []Country{
			{
				Code: "UKR",
				Name: "Ukraine",
			},
			{
				Code: "USA",
				Name: "United States of America",
			},
		}, countries)
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
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool.SqlxDB(),
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
		wallet := CreateWalletFixture(t, ctx, dbConnectionPool.SqlxDB(),
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
