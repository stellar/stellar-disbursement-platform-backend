package data

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_CountryModelGet(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	countryModel := &CountryModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns error when country is not found", func(t *testing.T) {
		_, err := countryModel.Get(ctx, "not-found")
		require.Error(t, err)
		require.Equal(t, ErrRecordNotFound, err)
	})

	t.Run("returns asset successfully", func(t *testing.T) {
		expected := CreateCountryFixture(t, ctx, dbConnectionPool, "FRA", "France")
		actual, err := countryModel.Get(ctx, "FRA")
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
	})
}

func Test_CountryModelGetAll(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	countryModel := &CountryModel{dbConnectionPool: dbConnectionPool}

	t.Run("returns all countries successfully", func(t *testing.T) {
		expected := ClearAndCreateCountryFixtures(t, ctx, dbConnectionPool)
		actual, err := countryModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, expected, actual)
	})

	t.Run("returns empty array when no countries", func(t *testing.T) {
		DeleteAllCountryFixtures(t, ctx, dbConnectionPool)
		actual, err := countryModel.GetAll(ctx)
		require.NoError(t, err)

		assert.Equal(t, []Country{}, actual)
	})
}
