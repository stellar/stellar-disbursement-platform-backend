package tenant

import (
	"context"
	"fmt"
	"testing"

	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
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

func AssertRegisteredAssetsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, expectedAssets []string) {
	var registeredAssets []string
	queryRegisteredAssets := `
		SELECT CONCAT(code, ':', issuer) FROM assets
	`
	err := dbConnectionPool.SelectContext(ctx, &registeredAssets, queryRegisteredAssets)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedAssets, registeredAssets)
}

func AssertRegisteredWalletsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, expectedWallets []string) {
	var registeredWallets []string
	queryRegisteredWallets := `
		SELECT name FROM wallets
	`
	err := dbConnectionPool.SelectContext(ctx, &registeredWallets, queryRegisteredWallets)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedWallets, registeredWallets)
}

func AssertRegisteredUserFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, userFirstName, userLastName, userEmail string) {
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

func CreateTenantFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, name string) *Tenant {
	tenantName := name
	if name == "" {
		name, err := utils.RandomString(56)
		require.NoError(t, err)
		tenantName = name
	}

	const query = `
		WITH create_tenant AS (
			INSERT INTO tenants 
				(name) 
			VALUES 
				($1) 
			ON CONFLICT DO NOTHING
			RETURNING *
		)
		SELECT 
			ct.id,
			ct.name,
			ct.status,
			ct.email_sender_type,
			ct.sms_sender_type,
			ct.enable_mfa,
			ct.enable_recaptcha,
			ct.created_at,
			ct.updated_at
		FROM
		 create_tenant ct
		`

	tnt := &Tenant{
		Name: tenantName,
	}

	err := sqlExec.QueryRowxContext(ctx, query, tnt.Name).Scan(&tnt.ID, &tnt.Name, &tnt.Status, &tnt.EmailSenderType, &tnt.SMSSenderType, &tnt.EnableMFA, &tnt.EnableReCAPTCHA, &tnt.CreatedAt, &tnt.UpdatedAt)
	require.NoError(t, err)

	return tnt
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

// TenantSchemaMatchTablesFixture asserts if the new tenant database schema has the tables passed by parameter.
func TenantSchemaMatchTablesFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, schemaName string, tableNames []string) {
	t.Helper()

	const q = `
		SELECT table_name FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name
	`

	var schemaTables []string
	err := dbConnectionPool.SelectContext(ctx, &schemaTables, q, schemaName)
	require.NoError(t, err)

	assert.ElementsMatch(t, tableNames, schemaTables)
}

func ApplyMigrationsForTenantFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, tenantName string) {
	t.Helper()

	m := NewManager(WithDatabase(dbConnectionPool))
	dsn, err := m.GetDSNForTenant(ctx, tenantName)
	require.NoError(t, err)

	_, err = db.Migrate(dsn, migrate.Up, 0, sdpmigrations.FS, db.StellarSDPMigrationsTableName)
	require.NoError(t, err)
	_, err = db.Migrate(dsn, migrate.Up, 0, authmigrations.FS, db.StellarAuthMigrationsTableName)
	require.NoError(t, err)
}
