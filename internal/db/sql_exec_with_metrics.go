package db

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
)

type QueryType string

const (
	DeleteQueryType    QueryType = "DELETE"
	InsertQueryType    QueryType = "INSERT"
	SelectQueryType    QueryType = "SELECT"
	UndefinedQueryType QueryType = "UNDEFINED"
	UpdateQueryType    QueryType = "UPDATE"
)

func NewSQLExecuterWithMetrics(sqlExec SQLExecuter, monitorServiceInterface monitor.MonitorServiceInterface) (*SQLExecuterWithMetrics, error) {
	return &SQLExecuterWithMetrics{
		SQLExecuter:             sqlExec,
		monitorServiceInterface: monitorServiceInterface,
	}, nil
}

// SQLExecuterWithMetrics is a wrapper around SQLExecuter that implements the monitoring service.
type SQLExecuterWithMetrics struct {
	SQLExecuter
	monitorServiceInterface monitor.MonitorServiceInterface
}

// make sure SQLExecuterWithMetrics implements SQLExecuter:
var _ SQLExecuter = (*SQLExecuterWithMetrics)(nil)

// monitorDBQueryDuration is a method that helps monitor the db query duration using the monitoring service.
func (sqlExec *SQLExecuterWithMetrics) monitorDBQueryDuration(duration time.Duration, query string, err error) {
	labels := monitor.DBQueryLabels{
		QueryType: string(getQueryType(query)),
	}
	errMetric := sqlExec.monitorServiceInterface.MonitorDBQueryDuration(duration, getMetricTag(err), labels)
	if errMetric != nil {
		log.Errorf("Error trying to monitor db query duration: %s", errMetric)
	}
}

// QueryContext is a wrapper around QueryerContext interface QueryContext that includes monitoring the db query.
func (sqlExec *SQLExecuterWithMetrics) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	then := time.Now()

	err := sqlExec.SQLExecuter.GetContext(ctx, dest, query, args...)

	duration := time.Since(then)

	sqlExec.monitorDBQueryDuration(duration, query, err)

	return err
}

// SelectContext is a wrapper around DBConnetionPool SelectContext that includes monitoring the db query.
func (sqlExec *SQLExecuterWithMetrics) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	then := time.Now()

	err := sqlExec.SQLExecuter.SelectContext(ctx, dest, query, args...)

	duration := time.Since(then)

	sqlExec.monitorDBQueryDuration(duration, query, err)

	return err
}

// ExecContext is a wrapper around DBConnetionPool ExecContext that includes monitoring the db query.
func (sqlExec *SQLExecuterWithMetrics) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	then := time.Now()

	result, err := sqlExec.SQLExecuter.ExecContext(ctx, query, args...)

	duration := time.Since(then)

	sqlExec.monitorDBQueryDuration(duration, query, err)

	return result, err
}

// QueryContext is a wrapper around QueryerContext interface QueryContext that includes monitoring the db query.
func (sqlExec *SQLExecuterWithMetrics) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	then := time.Now()

	rows, err := sqlExec.SQLExecuter.QueryContext(ctx, query, args...)

	duration := time.Since(then)

	sqlExec.monitorDBQueryDuration(duration, query, err)

	return rows, err
}

// QueryxContext is a wrapper around QueryerContext interface QueryxContext that includes monitoring the db query.
func (sqlExec *SQLExecuterWithMetrics) QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	then := time.Now()

	rows, err := sqlExec.SQLExecuter.QueryxContext(ctx, query, args...)

	duration := time.Since(then)

	sqlExec.monitorDBQueryDuration(duration, query, err)

	return rows, err
}

// QueryRowxContext is a wrapper around QueryerContext interface QueryRowxContext that includes monitoring the db query.
func (sqlExec *SQLExecuterWithMetrics) QueryRowxContext(ctx context.Context, query string, args ...interface{}) *sqlx.Row {
	then := time.Now()

	row := sqlExec.SQLExecuter.QueryRowxContext(ctx, query, args...)

	duration := time.Since(then)

	sqlExec.monitorDBQueryDuration(duration, query, row.Err())

	return row
}

// getMetricTag is a helper that returns the correct metric tag to be used in the monitoring service.
func getMetricTag(err error) monitor.MetricTag {
	if err != nil {
		return monitor.FailureQueryDurationTag
	}

	return monitor.SuccessfulQueryDurationTag
}

// getQueryType is a helper that return the type of query being executed.
func getQueryType(query string) QueryType {
	words := strings.Fields(strings.TrimSpace(query))
	for _, word := range []string{"DELETE", "INSERT", "SELECT", "UPDATE"} {
		if word == words[0] {
			return QueryType(word)
		}
	}
	// Fresh out of ideas.
	return UndefinedQueryType
}
