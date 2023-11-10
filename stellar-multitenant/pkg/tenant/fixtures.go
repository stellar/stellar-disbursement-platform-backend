package tenant

import (
	"context"
	"fmt"
	"testing"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func DeleteAllTenantsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) {
	q := "DELETE FROM tenants"
	_, err := dbConnectionPool.ExecContext(ctx, q)
	require.NoError(t, err)

	var schemasToDrop []string
	q = "SELECT schema_name FROM information_schema.schemata WHERE schema_name ILIKE 'sdp_%'"
	err = dbConnectionPool.SelectContext(ctx, &schemasToDrop, q)
	require.NoError(t, err)

	for _, schema := range schemasToDrop {
		q = fmt.Sprintf("DROP SCHEMA %s CASCADE", pq.QuoteIdentifier(schema))
		_, err = dbConnectionPool.ExecContext(ctx, q)
		require.NoError(t, err)
	}
}

func ResetTenantConfigFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, tenantID string) *Tenant {
	t.Helper()

	const q = `
		UPDATE tenants
		SET
			email_sender_type = DEFAULT, sms_sender_type = DEFAULT, sep10_signing_public_key = NULL,
			distribution_public_key = NULL, enable_mfa = DEFAULT, enable_recaptcha = DEFAULT,
			cors_allowed_origins = NULL, base_url = NULL, sdp_ui_base_url = NULL
		WHERE
			id = $1
		RETURNING *
	`

	var tnt Tenant
	err := dbConnectionPool.GetContext(ctx, &tnt, q, tenantID)
	require.NoError(t, err)

	return &tnt
}

func AssertRegisteredAssets(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, expectedAssets []string) {
	var registeredAssets []string
	queryRegisteredAssets := `
		SELECT CONCAT(code, ':', issuer) FROM assets
	`
	err := dbConnectionPool.SelectContext(ctx, &registeredAssets, queryRegisteredAssets)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedAssets, registeredAssets)
}

func AssertRegisteredWallets(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, expectedWallets []string) {
	var registeredWallets []string
	queryRegisteredWallets := `
		SELECT name FROM wallets
	`
	err := dbConnectionPool.SelectContext(ctx, &registeredWallets, queryRegisteredWallets)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedWallets, registeredWallets)
}

func AssertRegisteredUser(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, userFirstName, userLastName, userEmail string) {
	var user struct {
		FirstName string         `db:"first_name"`
		LastName  string         `db:"last_name"`
		Email     string         `db:"email"`
		Roles     pq.StringArray `db:"roles"`
		IsOwner   bool           `db:"is_owner"`
	}
	queryRegisteredUser := `
		SELECT first_name, last_name, email, roles, is_owner FROM auth_users WHERE email = $1
	`
	err := dbConnectionPool.GetContext(ctx, &user, queryRegisteredUser, userEmail)
	require.NoError(t, err)
	assert.Equal(t, userFirstName, user.FirstName)
	assert.Equal(t, userLastName, user.LastName)
	assert.Equal(t, userEmail, user.Email)
	assert.Equal(t, pq.StringArray{"owner"}, user.Roles)
	assert.True(t, user.IsOwner)
}

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
