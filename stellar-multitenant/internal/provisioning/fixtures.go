package provisioning

import (
	"context"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func CheckSchemaExistsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, schemaName string) bool {
	t.Helper()

	const q = `
		SELECT EXISTS(
			SELECT schema_name FROM information_schema.schemata WHERE schema_name = $1
		)
	`

	var exists bool
	err := dbConnectionPool.GetContext(ctx, &exists, q, schemaName)
	require.NoError(t, err)

	return exists
}

// TenantSchemaHasTablesFixture asserts if the new tenant database schema has the tables passed by parameter.
func TenantSchemaHasTablesFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, schemaName string, tableNames []string) {
	t.Helper()

	const q = `
		SELECT table_name FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name
	`

	var schemaTables []string
	err := dbConnectionPool.SelectContext(ctx, &schemaTables, q, schemaName)
	require.NoError(t, err)

	assert.ElementsMatch(t, tableNames, schemaTables)
}
