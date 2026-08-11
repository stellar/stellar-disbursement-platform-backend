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

// Test_WalletMemberships_backfill simulates a production UPGRADE: a tenant schema fully
// migrated up to (but excluding) the membership backfill, with real users and a default
// wallet, then the backfill applies. Every existing non-owner user must become a member of
// the default wallet with each of their current roles; owners get no rows.
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
	_, err = dbConnectionPool.ExecContext(ctx, `
		INSERT INTO distribution_wallets (name, distribution_account_type)
		VALUES ('secondary', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT')`)
	require.NoError(t, err)

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

	// Initiator: 1 row. Multi-role: 2 rows. Owner: none. All on the DEFAULT wallet only.
	// (Assertions are order-independent: user IDs are random UUIDs, so sort order varies.)
	require.Len(t, rows, 3)
	rolesByUser := map[string][]string{}
	for _, r := range rows {
		assert.Equal(t, defaultWalletID, r.WalletID, "backfill must target only the default wallet")
		assert.NotEqual(t, ownerID, r.UserID, "owners are tenant-wide and get no membership rows")
		rolesByUser[r.UserID] = append(rolesByUser[r.UserID], r.Role)
	}
	assert.ElementsMatch(t, []string{"initiator"}, rolesByUser[initiatorID])
	assert.ElementsMatch(t, []string{"approver", "financial_controller"}, rolesByUser[multiRoleID])
}
