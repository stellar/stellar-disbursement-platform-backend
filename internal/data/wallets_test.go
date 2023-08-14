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

		assert.Equal(t, expected, actual)
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

		assert.Equal(t, expected, actual)
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

	t.Run("returns all wallets successfully", func(t *testing.T) {
		expected := ClearAndCreateWalletFixtures(t, ctx, dbConnectionPool.SqlxDB())
		actual, err := walletModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
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

	t.Run("inserts wallet successfully", func(t *testing.T) {
		DeleteAllWalletFixtures(t, ctx, dbConnectionPool)
		name := "test_wallet"
		homepage := "https://www.test_wallet.com"
		deep_link_schema := "test_wallet://"
		sep_10_client_domain := "www.test_wallet.com"

		wallet, err := walletModel.Insert(ctx, name, homepage, deep_link_schema, sep_10_client_domain)
		require.NoError(t, err)
		assert.NotNil(t, wallet)

		insertedWallet, err := walletModel.Get(ctx, wallet.ID)
		require.NoError(t, err)
		assert.NotNil(t, insertedWallet)
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
