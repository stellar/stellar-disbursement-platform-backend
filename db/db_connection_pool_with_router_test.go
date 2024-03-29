package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionPoolWithRouter_BeginTxx(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	connectionPoolWithRouter, outerErr := NewConnectionPoolWithRouter(mockRouter)
	require.NoError(t, outerErr)
	ctx := context.Background()

	t.Run("BeginTxx successful", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		dbTx, err := connectionPoolWithRouter.BeginTxx(ctx, nil)

		// Defer a rollback in case anything fails.
		defer func() {
			err = dbTx.Rollback()
			require.Error(t, err, "not in transaction")
		}()
		require.NoError(t, err)

		assert.IsType(t, &sqlx.Tx{}, dbTx)

		err = dbTx.Commit()
		require.NoError(t, err)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(nil, assert.AnError).
			Once()

		dbTx, err := connectionPoolWithRouter.BeginTxx(ctx, nil)
		require.Error(t, err)
		assert.Nil(t, dbTx)

		mockRouter.AssertExpectations(t)
	})
}

func TestConnectionPoolWithRouter_Close(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	connectionPoolWithRouter, outerErr := NewConnectionPoolWithRouter(mockRouter)
	require.NoError(t, outerErr)

	t.Run("Close successful", func(t *testing.T) {
		mockRouter.
			On("GetAllDataSources").
			Return([]DBConnectionPool{dbConnectionPool}, nil).
			Once()

		err := connectionPoolWithRouter.Close()
		require.NoError(t, err)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting all data sources", func(t *testing.T) {
		mockRouter.
			On("GetAllDataSources").
			Return(nil, assert.AnError).
			Once()

		err := connectionPoolWithRouter.Close()
		require.Error(t, err)

		mockRouter.AssertExpectations(t)
	})
}

func TestConnectionPoolWithRouter_Ping(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	connectionPoolWithRouter, outerErr := NewConnectionPoolWithRouter(mockRouter)
	require.NoError(t, outerErr)
	ctx := context.Background()

	t.Run("Ping successful", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		err := connectionPoolWithRouter.Ping(ctx)
		require.NoError(t, err)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(nil, assert.AnError).
			Once()

		err := connectionPoolWithRouter.Ping(ctx)
		require.Error(t, err)

		mockRouter.AssertExpectations(t)
	})
}

func TestConnectionPoolWithRouter_SqlDB(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	connectionPoolWithRouter, outerErr := NewConnectionPoolWithRouter(mockRouter)
	require.NoError(t, outerErr)
	ctx := context.Background()

	t.Run("SqlDB successful", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		db, err := connectionPoolWithRouter.SqlDB(ctx)
		require.NoError(t, err)

		assert.IsType(t, &sql.DB{}, db)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(nil, assert.AnError).
			Once()

		db, err := connectionPoolWithRouter.SqlDB(ctx)
		require.Error(t, err)
		assert.Nil(t, db)

		mockRouter.AssertExpectations(t)
	})
}

func TestConnectionPoolWithRouter_SqlxDB(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	connectionPoolWithRouter, outerErr := NewConnectionPoolWithRouter(mockRouter)
	require.NoError(t, outerErr)
	ctx := context.Background()

	t.Run("SqlDB successful", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		db, err := connectionPoolWithRouter.SqlxDB(ctx)
		require.NoError(t, err)

		assert.IsType(t, &sqlx.DB{}, db)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(nil, assert.AnError).
			Once()

		db, err := connectionPoolWithRouter.SqlxDB(ctx)
		require.Error(t, err)
		assert.Nil(t, db)

		mockRouter.AssertExpectations(t)
	})
}

func TestConnectionPoolWithRouter_DSN(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	connectionPoolWithRouter, outerErr := NewConnectionPoolWithRouter(mockRouter)
	require.NoError(t, outerErr)
	ctx := context.Background()

	t.Run("DSN successful", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		dsn, err := connectionPoolWithRouter.DSN(ctx)
		require.NoError(t, err)

		assert.Equal(t, dbt.DSN, dsn)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(nil, assert.AnError).
			Once()

		dsn, err := connectionPoolWithRouter.DSN(ctx)
		require.Error(t, err)
		assert.Equal(t, "", dsn)

		mockRouter.AssertExpectations(t)
	})
}
