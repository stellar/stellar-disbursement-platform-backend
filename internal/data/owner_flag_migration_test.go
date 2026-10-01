package data

import (
	"context"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
)

const ownerFlagMigrationName = "2026-10-01.0-sync-owner-flag-with-owner-role.sql"

// Test_ownerFlagMigration seeds owner flag/role shapes in a tenant migrated up to just before the
// migration, applies it, and checks each user's flag, roles and default-wallet memberships.
func Test_ownerFlagMigration(t *testing.T) {
	ctx := context.Background()

	type ownerState struct {
		IsOwner bool           `db:"is_owner"`
		Roles   pq.StringArray `db:"roles"`
	}

	// prepare returns a tenant DB with auth fully migrated, sdp migrated up to the migration, and a default wallet.
	prepare := func(t *testing.T) (db.DBConnectionPool, string) {
		dbt := dbtest.OpenWithoutMigrations(t)
		t.Cleanup(dbt.Close)
		dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
		require.NoError(t, err)
		t.Cleanup(func() { dbConnectionPool.Close() })

		entries, err := fs.ReadDir(migrations.SDPMigrationRouter.FS, ".")
		require.NoError(t, err)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".sql") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		migrationIdx := sort.SearchStrings(names, ownerFlagMigrationName)
		require.Less(t, migrationIdx, len(names), "owner flag migration must exist")
		require.Equal(t, ownerFlagMigrationName, names[migrationIdx])

		n, err := db.Migrate(dbt.DSN, migrate.Up, migrationIdx, migrations.SDPMigrationRouter)
		require.NoError(t, err)
		require.Equal(t, migrationIdx, n)
		_, err = db.Migrate(dbt.DSN, migrate.Up, 0, migrations.AuthMigrationRouter)
		require.NoError(t, err)

		var defaultWalletID string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &defaultWalletID, `
			INSERT INTO distribution_wallets (name, distribution_account_type, is_default)
			VALUES ('default', 'DISTRIBUTION_ACCOUNT.STELLAR.DB_VAULT', TRUE) RETURNING id`))

		return dbConnectionPool, defaultWalletID
	}

	newUser := func(t *testing.T, dbConnectionPool db.DBConnectionPool, email string, isOwner, isActive bool, roles []string) string {
		var id string
		require.NoError(t, dbConnectionPool.GetContext(ctx, &id, `
			INSERT INTO auth_users (encrypted_password, email, first_name, last_name, is_owner, is_active, roles)
			VALUES ('x', $1, 'F', 'L', $2, $3, $4) RETURNING id`, email, isOwner, isActive, pq.Array(roles)))
		return id
	}

	applyMigration := func(t *testing.T, dbConnectionPool db.DBConnectionPool) {
		dsn, err := dbConnectionPool.DSN(ctx)
		require.NoError(t, err)
		n, err := db.Migrate(dsn, migrate.Up, 1, migrations.SDPMigrationRouter)
		require.NoError(t, err)
		require.Equal(t, 1, n)
	}

	assertOwnerState := func(t *testing.T, dbConnectionPool db.DBConnectionPool, userID string, wantIsOwner bool, wantRoles []string) {
		t.Helper()
		var got ownerState
		require.NoError(t, dbConnectionPool.GetContext(ctx, &got, "SELECT is_owner, roles FROM auth_users WHERE id = $1", userID))
		assert.Equal(t, wantIsOwner, got.IsOwner)
		assert.Equal(t, wantRoles, []string(got.Roles))
	}

	membershipRoles := func(t *testing.T, dbConnectionPool db.DBConnectionPool, userID, walletID string) []string {
		var roles []string
		require.NoError(t, dbConnectionPool.SelectContext(ctx, &roles,
			"SELECT role FROM wallet_memberships WHERE user_id = $1 AND wallet_id = $2 ORDER BY role", userID, walletID))
		return roles
	}

	t.Run("demotes flagged users with non-owner roles when another active owner remains", func(t *testing.T) {
		dbConnectionPool, walletID := prepare(t)
		ownerID := newUser(t, dbConnectionPool, "owner@example.com", true, true, []string{"owner"})
		demotedID := newUser(t, dbConnectionPool, "demoted@example.com", true, true, []string{"financial_controller"})
		inactiveDemotedID := newUser(t, dbConnectionPool, "inactive-demoted@example.com", true, false, []string{"business"})
		multiRoleOwnerID := newUser(t, dbConnectionPool, "multi-owner@example.com", false, true, []string{"business", "owner"})
		flagNoRolesID := newUser(t, dbConnectionPool, "flag-no-roles@example.com", true, true, nil)
		roleOnlyID := newUser(t, dbConnectionPool, "role-only@example.com", false, true, []string{"owner"})
		nonOwnerID := newUser(t, dbConnectionPool, "non-owner@example.com", false, true, []string{"business"})

		applyMigration(t, dbConnectionPool)

		assertOwnerState(t, dbConnectionPool, ownerID, true, []string{"owner"})
		assertOwnerState(t, dbConnectionPool, demotedID, false, []string{"financial_controller"})
		assertOwnerState(t, dbConnectionPool, inactiveDemotedID, false, []string{"business"})
		assertOwnerState(t, dbConnectionPool, multiRoleOwnerID, true, []string{"owner"})
		assertOwnerState(t, dbConnectionPool, flagNoRolesID, true, []string{"owner"})
		assertOwnerState(t, dbConnectionPool, roleOnlyID, true, []string{"owner"})
		assertOwnerState(t, dbConnectionPool, nonOwnerID, false, []string{"business"})

		assert.Equal(t, []string{"financial_controller"}, membershipRoles(t, dbConnectionPool, demotedID, walletID))
		assert.Equal(t, []string{"business"}, membershipRoles(t, dbConnectionPool, inactiveDemotedID, walletID))
		for _, id := range []string{ownerID, multiRoleOwnerID, flagNoRolesID, roleOnlyID, nonOwnerID} {
			assert.Empty(t, membershipRoles(t, dbConnectionPool, id, walletID))
		}
	})

	t.Run("a flagged user with no roles counts as the remaining owner", func(t *testing.T) {
		dbConnectionPool, walletID := prepare(t)
		newUser(t, dbConnectionPool, "flag-no-roles@example.com", true, true, nil)
		demotedID := newUser(t, dbConnectionPool, "demoted@example.com", true, true, []string{"approver"})

		applyMigration(t, dbConnectionPool)

		assertOwnerState(t, dbConnectionPool, demotedID, false, []string{"approver"})
		assert.Equal(t, []string{"approver"}, membershipRoles(t, dbConnectionPool, demotedID, walletID))
	})

	t.Run("keeps flagged users as owners when no other active owner remains", func(t *testing.T) {
		dbConnectionPool, walletID := prepare(t)
		newUser(t, dbConnectionPool, "inactive-owner@example.com", true, false, []string{"owner"})
		keptID := newUser(t, dbConnectionPool, "kept@example.com", true, true, []string{"financial_controller"})

		applyMigration(t, dbConnectionPool)

		assertOwnerState(t, dbConnectionPool, keptID, true, []string{"owner"})
		assert.Empty(t, membershipRoles(t, dbConnectionPool, keptID, walletID))
	})

	t.Run("down is a no-op that can still be applied", func(t *testing.T) {
		dbConnectionPool, _ := prepare(t)
		applyMigration(t, dbConnectionPool)

		dsn, err := dbConnectionPool.DSN(ctx)
		require.NoError(t, err)
		n, err := db.Migrate(dsn, migrate.Down, 1, migrations.SDPMigrationRouter)
		require.NoError(t, err)
		assert.Equal(t, 1, n)
	})
}
