package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSQLExecWithMetrics_GetContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	sqlExecWithMetrics, err := NewSQLExecuterWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()
	var mDest string

	const query = `
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
	`
	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	t.Run("query successful in GetContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "SELECT",
		}
		mQuery := "SELECT a.code FROM assets a WHERE a.issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC'"

		mMonitorService.On(
			"MonitorDBQueryDuration",
			mock.AnythingOfType("time.Duration"),
			monitor.SuccessfulQueryDurationTag,
			mLabels,
		).Return(nil).Once()

		err := sqlExecWithMetrics.GetContext(ctx, &mDest, mQuery)
		require.NoError(t, err)

		expected := "USDC"
		assert.Equal(t, expected, mDest)

		mMonitorService.AssertExpectations(t)
	})

	t.Run("query failure in GetContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "SELECT",
		}
		mQuery := "SELECT a.code FROM assets a WHERE a.issuer = 'invalid_issuer'"

		mMonitorService.On("MonitorDBQueryDuration", mock.AnythingOfType("time.Duration"), monitor.FailureQueryDurationTag, mLabels).Return(nil).Once()

		err := sqlExecWithMetrics.GetContext(ctx, &mDest, mQuery)
		require.EqualError(t, err, "sql: no rows in result set")

		mMonitorService.AssertExpectations(t)
	})
}

func TestSQLExecWithMetrics_SelectContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	sqlExecWithMetrics, err := NewSQLExecuterWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()
	var mDest []string

	const query = `
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
	`
	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "EURT", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	t.Run("query successful in SelectContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "SELECT",
		}
		mQuery := "SELECT a.code FROM assets a WHERE a.issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC'"

		mMonitorService.On(
			"MonitorDBQueryDuration",
			mock.AnythingOfType("time.Duration"),
			monitor.SuccessfulQueryDurationTag,
			mLabels,
		).Return(nil).Once()

		err := sqlExecWithMetrics.SelectContext(ctx, &mDest, mQuery)
		require.NoError(t, err)

		expected := []string{"USDC", "EURT"}
		assert.Equal(t, expected, mDest)

		mMonitorService.AssertExpectations(t)
	})

	t.Run("query failure in SelectContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "UNDEFINED",
		}
		mQuery := "invalid query"

		mMonitorService.On("MonitorDBQueryDuration", mock.AnythingOfType("time.Duration"), monitor.FailureQueryDurationTag, mLabels).Return(nil).Once()

		err := sqlExecWithMetrics.SelectContext(ctx, &mDest, mQuery)
		require.EqualError(t, err, `pq: syntax error at or near "invalid"`)

		mMonitorService.AssertExpectations(t)
	})
}

func TestSQLExecWithMetrics_QueryContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	sqlExecWithMetrics, err := NewSQLExecuterWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()

	const query = `
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
	`
	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "EURT", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	t.Run("query successful in QueryContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "SELECT",
		}
		mQuery := "SELECT a.code FROM assets a WHERE a.issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC'"

		mMonitorService.On(
			"MonitorDBQueryDuration",
			mock.AnythingOfType("time.Duration"),
			monitor.SuccessfulQueryDurationTag,
			mLabels,
		).Return(nil).Once()

		rows, err := sqlExecWithMetrics.QueryContext(ctx, mQuery)
		require.NoError(t, err)
		defer rows.Close()

		expected := []string{"USDC", "EURT"}
		for rows.Next() {
			var code string
			err := rows.Scan(&code)
			require.NoError(t, err)

			assert.Contains(t, expected, code)
		}

		mMonitorService.AssertExpectations(t)
	})

	t.Run("query failure in QueryContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "UNDEFINED",
		}
		mQuery := "invalid query"

		mMonitorService.On("MonitorDBQueryDuration", mock.AnythingOfType("time.Duration"), monitor.FailureQueryDurationTag, mLabels).Return(nil).Once()

		rows, err := sqlExecWithMetrics.QueryContext(ctx, mQuery)
		require.EqualError(t, err, `pq: syntax error at or near "invalid"`)

		assert.Nil(t, rows)

		mMonitorService.AssertExpectations(t)
	})
}

func TestSQLExecWithMetrics_QueryxContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	sqlExecWithMetrics, err := NewSQLExecuterWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()

	const query = `
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
	`
	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "EURT", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	t.Run("query successful in QueryxContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "SELECT",
		}
		mQuery := "SELECT a.code FROM assets a WHERE a.issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC'"

		mMonitorService.On(
			"MonitorDBQueryDuration",
			mock.AnythingOfType("time.Duration"),
			monitor.SuccessfulQueryDurationTag,
			mLabels,
		).Return(nil).Once()

		rows, err := sqlExecWithMetrics.QueryxContext(ctx, mQuery)
		require.NoError(t, err)
		defer rows.Close()

		expected := []string{"USDC", "EURT"}
		for rows.Next() {
			var code string
			err := rows.Scan(&code)
			require.NoError(t, err)

			assert.Contains(t, expected, code)
		}

		mMonitorService.AssertExpectations(t)
	})

	t.Run("query failure in QueryxContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "UNDEFINED",
		}
		mQuery := "invalid query"

		mMonitorService.On("MonitorDBQueryDuration", mock.AnythingOfType("time.Duration"), monitor.FailureQueryDurationTag, mLabels).Return(nil).Once()

		rows, err := sqlExecWithMetrics.QueryxContext(ctx, mQuery)
		require.EqualError(t, err, `pq: syntax error at or near "invalid"`)

		assert.Nil(t, rows)

		mMonitorService.AssertExpectations(t)
	})
}

func TestSQLExecWithMetrics_QueryRowxContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	sqlExecWithMetrics, err := NewSQLExecuterWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()

	const query = `
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
	`
	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	t.Run("query successful in QueryRowxContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "SELECT",
		}
		mQuery := "SELECT a.code FROM assets a WHERE a.issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC'"

		mMonitorService.On(
			"MonitorDBQueryDuration",
			mock.AnythingOfType("time.Duration"),
			monitor.SuccessfulQueryDurationTag,
			mLabels,
		).Return(nil).Once()

		var code string
		err := sqlExecWithMetrics.QueryRowxContext(ctx, mQuery).Scan(&code)
		require.NoError(t, err)

		expected := "USDC"
		assert.Contains(t, expected, code)

		mMonitorService.AssertExpectations(t)
	})

	t.Run("query failure in QueryRowxContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "UNDEFINED",
		}
		mQuery := "invalid query"

		mMonitorService.On("MonitorDBQueryDuration", mock.AnythingOfType("time.Duration"), monitor.FailureQueryDurationTag, mLabels).Return(nil).Once()

		var code string
		err := sqlExecWithMetrics.QueryRowxContext(ctx, mQuery).Scan(&code)
		require.EqualError(t, err, `pq: syntax error at or near "invalid"`)

		mMonitorService.AssertExpectations(t)
	})
}

func TestSQLExecWithMetrics_ExecContext(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	mMonitorService := &monitor.MockMonitorService{}

	sqlExecWithMetrics, err := NewSQLExecuterWithMetrics(dbConnectionPool, mMonitorService)
	require.NoError(t, err)

	ctx := context.Background()
	const query = `
		INSERT INTO assets
			(code, issuer)
		VALUES
			($1, $2)
	`
	_, err = dbConnectionPool.SqlDB().ExecContext(ctx, query, "USDC", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC")
	require.NoError(t, err)

	t.Run("query successful in ExecContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "UPDATE",
		}
		mQuery := "UPDATE assets SET code = $1 WHERE issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC'"

		mMonitorService.On(
			"MonitorDBQueryDuration",
			mock.AnythingOfType("time.Duration"),
			monitor.SuccessfulQueryDurationTag,
			mLabels,
		).Return(nil).Once()

		result, err := sqlExecWithMetrics.ExecContext(ctx, mQuery, "EURT")
		require.NoError(t, err)

		rowsAffected, err := result.RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, rowsAffected, int64(1))

		mMonitorService.AssertExpectations(t)
	})

	t.Run("query failure in ExecContext", func(t *testing.T) {
		mLabels := monitor.DBQueryLabels{
			QueryType: "UPDATE",
		}
		mQuery := "UPDATE invalid_table SET code = $1 WHERE issuer = 'GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZCC'"

		mMonitorService.On("MonitorDBQueryDuration", mock.AnythingOfType("time.Duration"), monitor.FailureQueryDurationTag, mLabels).Return(nil).Once()

		_, err := sqlExecWithMetrics.ExecContext(ctx, mQuery, "EURT")
		require.EqualError(t, err, `pq: relation "invalid_table" does not exist`)

		mMonitorService.AssertExpectations(t)
	})
}

func TestSQLExecWithMetrics_getMetricTag(t *testing.T) {
	t.Run("return successful metric tag", func(t *testing.T) {
		metricTag := getMetricTag(nil)

		assert.Equal(t, monitor.SuccessfulQueryDurationTag, metricTag)
	})

	t.Run("return failure metric tag", func(t *testing.T) {
		metricTag := getMetricTag(fmt.Errorf("get failed"))

		assert.Equal(t, monitor.FailureQueryDurationTag, metricTag)
	})
}

func TestSQLExecWithMetrics_getQueryType(t *testing.T) {
	testCases := []struct {
		query             string
		expectedQueryType QueryType
	}{
		{query: "SELECT * FROM mock_table", expectedQueryType: SelectQueryType},
		{query: "UPDATE mock_table SET mock = 'mock' WHERE id = 1", expectedQueryType: UpdateQueryType},
		{query: "INSERT INTO mock_table (id) VALUES (1)", expectedQueryType: InsertQueryType},
		{query: "DELETE FROM mock_table WHERE id = 1", expectedQueryType: DeleteQueryType},
		{query: "invalid query", expectedQueryType: UndefinedQueryType},
	}
	for _, tc := range testCases {
		t.Run("get query type for query: "+tc.query, func(t *testing.T) {
			queryType := getQueryType(tc.query)

			assert.Equal(t, tc.expectedQueryType, queryType)
		})
	}
}
