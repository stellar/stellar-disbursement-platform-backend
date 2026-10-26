package data

import (
	"context"
	"io/fs"
	"sort"
	"strings"
	"testing"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
)

const backfillMigrationName = "2026-06-09.5-backfill-wallet-memberships.sql"

// Test_WalletMemberships_backfill upgrades a schema from just before the backfill: scoped users join the default wallet
// with their roles, tenant-wide users (owners, developers) end with no rows, and developer grants are removed.
func Test_WalletMemberships_backfill(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	// Count sdp migrations strictly before the backfill (order-robust even if later
	// migrations are appended after it).
	entries, err := fs.ReadDir(migrations.SDPMigrationRouter.FS, ".")
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	backfillIdx := sort.SearchStrings(names, backfillMigrationName)
	require.Less(t, backfillIdx, len(names), "backfill migration must exist")
	require.Equal(t, backfillMigrationName, names[backfillIdx])

	// 1. Pre-upgrade state: sdp up to the backfill, auth fully applied.
	n, err := db.Migrate(dbt.DSN, migrate.Up, backfillIdx, migrations.SDPMigrationRouter)
	require.NoError(t, err)
	require.Equal(t, backfillIdx, n)
	_, err = db.Migrate(dbt.DSN, migrate.Up, 0, migrations.AuthMigrationRouter)
	require.NoError(t, err)

	// 2. Real-world data: a default wallet, a second wallet, three users.
	var defaultWalletID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &defaultWalletID, `
		INSERT INTO distribution_wallets (name, distribution_account_type, is_default)
		VALUES ('default', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', TRUE) RETURNING id`))
	var secondaryWalletID string
	require.NoError(t, dbConnectionPool.GetContext(ctx, &secondaryWalletID, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('secondary', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT') RETURNING id`))

	newUser := func(email string, isOwner bool, roles string) string {
		var id string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &id, `
			INSERT INTO auth_users (encrypted_password, email, first_name, last_name, is_owner, roles)
			VALUES ('x', $1, 'F', 'L', $2, $3::text[]) RETURNING id`, email, isOwner, roles))
		return id
	}
	ownerID := newUser("owner@example.com", true, `{owner}`)
	initiatorID := newUser("initiator@example.com", false, `{initiator}`)
	multiRoleID := newUser("multi@example.com", false, `{financial_controller,approver}`)
	developerID := newUser("developer@example.com", false, `{developer}`)
	// A dashboard-demoted owner: the owner-flag sync grants a developer membership, which must run before it is removed.
	flaggedDeveloperID := newUser("flagged-developer@example.com", true, `{developer}`)
	businessID := newUser("business@example.com", false, `{business}`)
	_, err = dbConnectionPool.ExecContext(ctx, `
		INSERT INTO wallet_memberships (user_id, wallet_id, role) VALUES ($1, $2, 'developer')`, businessID, secondaryWalletID)
	require.NoError(t, err)
	// A tenant-wide user's rows go whatever their role: a developer's, and an owner's from before they were promoted.
	_, err = dbConnectionPool.ExecContext(ctx, `
		INSERT INTO wallet_memberships (user_id, wallet_id, role) VALUES ($1, $3, 'financial_controller'), ($2, $3, 'business')`,
		developerID, ownerID, secondaryWalletID)
	require.NoError(t, err)

	// 3. The upgrade: apply the remaining sdp migrations (the backfill runs now).
	_, err = db.Migrate(dbt.DSN, migrate.Up, 0, migrations.SDPMigrationRouter)
	require.NoError(t, err)

	type row struct {
		UserID   string `db:"user_id"`
		WalletID string `db:"wallet_id"`
		Role     string `db:"role"`
	}
	var rows []row
	require.NoError(t, dbConnectionPool.SelectContext(ctx, &rows, `
		SELECT user_id, wallet_id, role FROM wallet_memberships ORDER BY user_id, role`))

	// Initiator 1, multi-role 2, business 1 (its developer grant is gone), all on the default wallet.
	// (Assertions are order-independent: user IDs are random UUIDs, so sort order varies.)
	require.Len(t, rows, 4)
	rolesByUser := map[string][]string{}
	for _, r := range rows {
		assert.Equal(t, defaultWalletID, r.WalletID, "backfill must target only the default wallet")
		for _, tenantWideID := range []string{ownerID, developerID, flaggedDeveloperID} {
			assert.NotEqual(t, tenantWideID, r.UserID, "tenant-wide users get no membership rows")
		}
		rolesByUser[r.UserID] = append(rolesByUser[r.UserID], r.Role)
	}
	assert.ElementsMatch(t, []string{"initiator"}, rolesByUser[initiatorID])
	assert.ElementsMatch(t, []string{"approver", "financial_controller"}, rolesByUser[multiRoleID])
	assert.ElementsMatch(t, []string{"business"}, rolesByUser[businessID])

	var flaggedDeveloperIsOwner bool
	require.NoError(t, dbConnectionPool.GetContext(ctx, &flaggedDeveloperIsOwner,
		`SELECT is_owner FROM auth_users WHERE id = $1`, flaggedDeveloperID))
	assert.False(t, flaggedDeveloperIsOwner, "the owner-flag sync demotes a flagged developer")
}
