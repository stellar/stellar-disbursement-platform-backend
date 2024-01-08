package db

import (
	"context"
	"fmt"
	"net/url"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	tssmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/tss-migrations"
)

type TSSDatabaseMigrationManager struct {
	RootDatabaseDSN      string
	RootDBConnectionPool db.DBConnectionPool
}

func (m *TSSDatabaseMigrationManager) SchemaName() string {
	return "tss"
}

func NewTSSDatabaseMigrationManager(rootDatabaseDSN string) (*TSSDatabaseMigrationManager, error) {
	rootDBConnectionPool, err := db.OpenDBConnectionPool(rootDatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("opening root database connection pool: %w", err)
	}

	return &TSSDatabaseMigrationManager{
		RootDatabaseDSN:      rootDatabaseDSN,
		RootDBConnectionPool: rootDBConnectionPool,
	}, nil
}

func (m *TSSDatabaseMigrationManager) createTSSSchemaIfNeeded(ctx context.Context) error {
	query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", m.SchemaName())
	_, err := m.RootDBConnectionPool.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("creating the '%s' database schema: %w", m.SchemaName(), err)
	}

	return nil
}

func (m *TSSDatabaseMigrationManager) getTSSDatabaseDSN() (string, error) {
	dbURL, err := url.Parse(m.RootDatabaseDSN)
	if err != nil {
		return "", fmt.Errorf("parsing database DSN: %w", err)
	}

	q := dbURL.Query()
	q.Set("search_path", m.SchemaName())
	dbURL.RawQuery = q.Encode()

	return dbURL.String(), nil
}

func (m *TSSDatabaseMigrationManager) deleteTSSSchemaIfNeeded(ctx context.Context) error {
	// Delete the `tss` schema if needed.
	var numberOfRemainingTablesInTSSSchema int
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = '%s'
		AND table_name NOT LIKE '%%migrations%%'
	`, m.SchemaName())

	err := m.RootDBConnectionPool.GetContext(ctx, &numberOfRemainingTablesInTSSSchema, query)
	if err != nil {
		return fmt.Errorf("counting number of tables remaining in the '%s' database schema: %w", m.SchemaName(), err)
	}

	if numberOfRemainingTablesInTSSSchema == 0 {
		log.Ctx(ctx).Infof("dropping the '%s' database schema ⏳...", m.SchemaName())
		query := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", m.SchemaName())
		_, err = m.RootDBConnectionPool.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("dropping the '%s' database schema: %w", m.SchemaName(), err)
		}
		log.Ctx(ctx).Infof("dropped the '%s' database schema ✅", m.SchemaName())
	} else {
		log.Ctx(ctx).Debugf("the '%s' database schema was not dropped because there are %d tables remaining", m.SchemaName(), numberOfRemainingTablesInTSSSchema)
	}

	return nil
}

func runTSSMigrations(ctx context.Context, dbURL string, dir migrate.MigrationDirection, count int) error {
	err := ExecuteMigrations(ctx, dbURL, dir, count, tssmigrations.FS, db.StellarTSSMigrationsTableName)
	if err != nil {
		return fmt.Errorf("executing TSS migrations: %w", err)
	}

	return nil
}
