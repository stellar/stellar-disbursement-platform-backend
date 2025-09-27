package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

type MockDataSourceRouter struct {
	mock.Mock
}

func (m *MockDataSourceRouter) GetAllDataSources() ([]DBConnectionPool, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]DBConnectionPool), args.Error(1)
}

func (m *MockDataSourceRouter) AnyDataSource() (DBConnectionPool, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(DBConnectionPool), args.Error(1)
}

func (m *MockDataSourceRouter) GetDataSource(ctx context.Context) (DBConnectionPool, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(DBConnectionPool), args.Error(1)
}

func TestSQLExecutorWithRouter_GetContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	ctx := context.Background()
	query := "SELECT o.name FROM organizations o"
	var dest string

	t.Run("query successful in GetContext", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		err := sqlExecWithRouter.GetContext(ctx, &dest, query)
		require.NoError(t, err)
		require.Equal(t, "MyCustomAid", dest)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source in GetContext", func(t *testing.T) {
		mockRouter.On("GetDataSource", ctx).
			Return(nil, fmt.Errorf("data source error")).
			Once()

		err := sqlExecWithRouter.GetContext(ctx, &dest, query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getting data source from context in GetContext")

		mockRouter.AssertExpectations(t)
	})
}

func TestSQLExecutorWithRouter_SelectContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	ctx := context.Background()
	query := "SELECT o.name FROM organizations o"
	var dest []string
	t.Run("query successful in SelectContext", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		err := sqlExecWithRouter.SelectContext(ctx, &dest, query)
		require.NoError(t, err)
		require.Equal(t, 1, len(dest))
		require.Equal(t, "MyCustomAid", dest[0])

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source in SelectContext", func(t *testing.T) {
		mockRouter.On("GetDataSource", ctx).
			Return(nil, fmt.Errorf("data source error")).
			Once()

		err := sqlExecWithRouter.SelectContext(ctx, &dest, query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getting data source from context in SelectContext")

		mockRouter.AssertExpectations(t)
	})
}

func TestSQLExecutorWithRouter_ExecContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	ctx := context.Background()
	query := "INSERT INTO assets (code, issuer) VALUES ('BTC', 'GCNSGHUCG5VMGLT5RIYYZSO7VQULQKAJ62QA33DBC5PPBSO57LFWVV6P')"
	t.Run("query successful in ExecContext", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		result, err := sqlExecWithRouter.ExecContext(ctx, query)
		require.NoError(t, err)

		rowsAffected, err := result.RowsAffected()
		require.NoError(t, err)

		assert.Equal(t, rowsAffected, int64(1))
		require.NoError(t, err)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source in ExecContext", func(t *testing.T) {
		mockRouter.On("GetDataSource", ctx).
			Return(nil, fmt.Errorf("data source error")).
			Once()

		_, err := sqlExecWithRouter.ExecContext(ctx, query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getting data source from context in ExecContext")

		mockRouter.AssertExpectations(t)
	})
}

func TestSQLExecutorWithRouter_QueryContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	ctx := context.Background()
	query := "SELECT o.name FROM organizations o"
	t.Run("query successful in QueryContext", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		rows, err := sqlExecWithRouter.QueryContext(ctx, query)
		require.NoError(t, err)

		var dest []string
		for rows.Next() {
			var name string
			err = rows.Scan(&name)
			require.NoError(t, err)
			dest = append(dest, name)
		}
		require.NoError(t, rows.Err())

		require.Equal(t, 1, len(dest))
		require.Equal(t, "MyCustomAid", dest[0])

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source in QueryContext", func(t *testing.T) {
		mockRouter.On("GetDataSource", ctx).
			Return(nil, fmt.Errorf("data source error")).
			Once()

		rows, err := sqlExecWithRouter.QueryContext(ctx, query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getting data source from context in QueryContext")
		if rows != nil {
			require.NoError(t, rows.Err())
		}

		mockRouter.AssertExpectations(t)
	})
}

func TestSQLExecutorWithRouter_QueryxContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	ctx := context.Background()
	query := "SELECT o.name FROM organizations o"
	t.Run("query successful in QueryxContext", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		rows, err := sqlExecWithRouter.QueryxContext(ctx, query)
		require.NoError(t, err)

		var dest []string
		for rows.Next() {
			var name string
			err = rows.Scan(&name)
			require.NoError(t, err)
			dest = append(dest, name)
		}

		require.Equal(t, 1, len(dest))
		require.Equal(t, "MyCustomAid", dest[0])

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source in QueryxContext", func(t *testing.T) {
		mockRouter.On("GetDataSource", ctx).
			Return(nil, fmt.Errorf("data source error")).
			Once()

		_, err := sqlExecWithRouter.QueryxContext(ctx, query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getting data source from context in QueryxContext")

		mockRouter.AssertExpectations(t)
	})
}

func TestSQLExecutorWithRouter_PrepareContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	ctx := context.Background()
	query := "SELECT o.name FROM organizations o"
	t.Run("query successful in PrepareContext", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		stmt, err := sqlExecWithRouter.PrepareContext(ctx, query)
		require.NoError(t, err)

		rows, err := stmt.Query()
		require.NoError(t, err)

		var dest []string
		for rows.Next() {
			var name string
			err = rows.Scan(&name)
			require.NoError(t, err)
			dest = append(dest, name)
		}
		require.NoError(t, rows.Err())

		require.Equal(t, 1, len(dest))
		require.Equal(t, "MyCustomAid", dest[0])

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source in PrepareContext", func(t *testing.T) {
		mockRouter.On("GetDataSource", ctx).
			Return(nil, fmt.Errorf("data source error")).
			Once()

		_, err := sqlExecWithRouter.PrepareContext(ctx, query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "getting data source from context in PrepareContext")

		mockRouter.AssertExpectations(t)
	})
}

func TestSQLExecutorWithRouter_QueryRowxContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	ctx := context.Background()
	query := "SELECT o.name FROM organizations o"
	t.Run("query successful in QueryRowxContext", func(t *testing.T) {
		mockRouter.
			On("GetDataSource", ctx).
			Return(dbConnectionPool, nil).
			Once()

		row := sqlExecWithRouter.QueryRowxContext(ctx, query)

		var dest string
		err := row.Scan(&dest)
		require.NoError(t, err)

		require.Equal(t, "MyCustomAid", dest)

		mockRouter.AssertExpectations(t)
	})

	t.Run("error getting data source in QueryRowxContext", func(t *testing.T) {
		mockRouter.On("GetDataSource", ctx).
			Return(nil, fmt.Errorf("data source error")).
			Once()

		row := sqlExecWithRouter.QueryRowxContext(ctx, query)
		require.Nil(t, row)

		mockRouter.AssertExpectations(t)
	})
}

func TestSQLExecutorWithRouter_Rebind(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	query := "SELECT * FROM organizations o WHERE o.name = ?"
	expected := "SELECT * FROM organizations o WHERE o.name = $1"
	t.Run("query successful in Rebind", func(t *testing.T) {
		mockRouter.
			On("AnyDataSource").
			Return(dbConnectionPool, nil).
			Once()
		reboundQuery := sqlExecWithRouter.Rebind(query)
		require.Equal(t, expected, reboundQuery)
	})

	t.Run("query successful in Rebind when there is no connectionPool", func(t *testing.T) {
		mockRouter.
			On("AnyDataSource").
			Return(nil, fmt.Errorf("data source error")).
			Once()
		reboundQuery := sqlExecWithRouter.Rebind(query)
		require.Equal(t, expected, reboundQuery)
	})
}

func TestSQLExecutorWithRouter_DriverName(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	mockRouter := new(MockDataSourceRouter)

	sqlExecWithRouter, outerErr := NewSQLExecutorWithRouter(mockRouter)
	require.NoError(t, outerErr)

	expected := "postgres"
	t.Run("query successful in DriverName", func(t *testing.T) {
		mockRouter.
			On("AnyDataSource").
			Return(dbConnectionPool, nil).
			Once()
		driverName := sqlExecWithRouter.DriverName()
		require.Equal(t, expected, driverName)
	})

	t.Run("empty when there is no connection pool", func(t *testing.T) {
		mockRouter.
			On("AnyDataSource").
			Return(nil, fmt.Errorf("data source error")).
			Once()
		driverName := sqlExecWithRouter.DriverName()
		require.Empty(t, driverName)
	})
}
