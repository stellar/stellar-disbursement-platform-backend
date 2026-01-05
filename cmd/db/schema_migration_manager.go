package db

import (
	"context"
	"fmt"
	"io"
	"strings"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type SchemaMigrationManager struct {
	MigrationRouter        migrations.MigrationRouter
	SchemaName             string
	SchemaDatabaseDSN      string
	schemaDBConnectionPool db.DBConnectionPool
}

var _ io.Closer = (*SchemaMigrationManager)(nil)

func (m *SchemaMigrationManager) Close() error {
	return m.schemaDBConnectionPool.Close()
}

func NewSchemaMigrationManager(
	migrationRouter migrations.MigrationRouter,
	schemaName string,
	schemaDatabaseDSN string,
) (*SchemaMigrationManager, error) {
	if utils.IsEmpty(migrationRouter) {
		return nil, fmt.Errorf("migrationRouter cannot be empty")
	}

	if strings.TrimSpace(schemaName) == "" {
		return nil, fmt.Errorf("schemaName cannot be empty")
	}

	if strings.TrimSpace(schemaDatabaseDSN) == "" {
		return nil, fmt.Errorf("schemaDatabaseDSN cannot be empty")
	}

	schemaDBConnectionPool, err := db.OpenDBConnectionPool(schemaDatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("opening the database connection pool for the '%s' schema: %w", schemaName, err)
	}

	return &SchemaMigrationManager{
		MigrationRouter:        migrationRouter,
		SchemaName:             schemaName,
		SchemaDatabaseDSN:      schemaDatabaseDSN,
		schemaDBConnectionPool: schemaDBConnectionPool,
	}, nil
}

func (m *SchemaMigrationManager) createSchemaIfNeeded(ctx context.Context) error {
	query := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", m.SchemaName)
	_, err := m.schemaDBConnectionPool.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("creating the '%s' database schema: %w", m.SchemaName, err)
	}

	return nil
}

func (m *SchemaMigrationManager) deleteSchemaIfNeeded(ctx context.Context) error {
	var numberOfRemainingTablesInSchema int
	query := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = '%s'
		AND table_name NOT LIKE '%%migrations%%'
	`, m.SchemaName)

	err := m.schemaDBConnectionPool.GetContext(ctx, &numberOfRemainingTablesInSchema, query)
	if err != nil {
		return fmt.Errorf("counting number of tables remaining in the '%s' database schema: %w", m.SchemaName, err)
	}

	if numberOfRemainingTablesInSchema == 0 {
		log.Ctx(ctx).Infof("dropping the '%s' database schema ⏳...", m.SchemaName)
		query := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", m.SchemaName)
		_, err = m.schemaDBConnectionPool.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("dropping the '%s' database schema: %w", m.SchemaName, err)
		}
		log.Ctx(ctx).Infof("dropped the '%s' database schema ✅", m.SchemaName)
	} else {
		log.Ctx(ctx).Debugf("the '%s' database schema was not dropped because there are %d tables remaining", m.SchemaName, numberOfRemainingTablesInSchema)
	}

	return nil
}

func (m *SchemaMigrationManager) executeMigrations(ctx context.Context, dir migrate.MigrationDirection, count int) error {
	err := ExecuteMigrations(ctx, m.SchemaDatabaseDSN, dir, count, m.MigrationRouter)
	if err != nil {
		return fmt.Errorf("executing migrations for router %s: %w", m.MigrationRouter.TableName, err)
	}

	return nil
}

func (m *SchemaMigrationManager) OrchestrateSchemaMigrations(ctx context.Context, dir migrate.MigrationDirection, count int) error {
	if err := m.createSchemaIfNeeded(ctx); err != nil {
		return fmt.Errorf("creating the '%s' database schema if needed: %w", m.SchemaName, err)
	}

	if err := m.executeMigrations(ctx, dir, count); err != nil {
		return fmt.Errorf("running migrations for the '%s' database schema: %w", m.SchemaName, err)
	}

	if err := m.deleteSchemaIfNeeded(ctx); err != nil {
		return fmt.Errorf("deleting the '%s' database schema if needed: %w", m.SchemaName, err)
	}

	return nil
}
