package dbtest

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	db := Open(t)
	defer db.Close()
	session := db.Open()
	defer session.Close()

	count := 0

	// Tenant migrations
	err := session.Get(&count, `SELECT COUNT(*) FROM admin_migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	// Per-tenant SDP migrations
	err = session.Get(&count, `SELECT COUNT(*) FROM sdp_migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	// Per-tenant Auth Migrations
	err = session.Get(&count, `SELECT COUNT(*) FROM auth_migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0)

	// Per-tenant TSS Migrations
	err = session.Get(&count, `SELECT COUNT(*) FROM tss_migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0)
}

func TestOpenWithAdminMigrationsOnly(t *testing.T) {
	db := OpenWithAdminMigrationsOnly(t)
	defer db.Close()
	session := db.Open()
	defer session.Close()

	count := 0
	err := session.Get(&count, `SELECT COUNT(*) FROM admin_migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0, "admin migrations should be applied")

	assert.False(t, tableExists(t, session, "sdp_migrations"), "sdp migrations should not be applied")
	assert.False(t, tableExists(t, session, "auth_migrations"), "auth migrations should not be applied")
	assert.False(t, tableExists(t, session, "tss_migrations"), "tss migrations should not be applied")
}

func TestOpenWithTSSMigrationsOnly(t *testing.T) {
	db := OpenWithTSSMigrationsOnly(t)
	defer db.Close()
	session := db.Open()
	defer session.Close()

	count := 0
	err := session.Get(&count, `SELECT COUNT(*) FROM tss_migrations`)
	require.NoError(t, err)
	assert.Greater(t, count, 0, "tss migrations should be applied")

	assert.False(t, tableExists(t, session, "admin_migrations"), "admin migrations should not be applied")
	assert.False(t, tableExists(t, session, "sdp_migrations"), "sdp migrations should not be applied")
	assert.False(t, tableExists(t, session, "auth_migrations"), "auth migrations should not be applied")
}

func TestOpenWithoutMigrations(t *testing.T) {
	db := OpenWithoutMigrations(t)
	defer db.Close()
	session := db.Open()
	defer session.Close()

	assert.False(t, tableExists(t, session, "admin_migrations"), "no migrations should be applied")
	assert.False(t, tableExists(t, session, "sdp_migrations"), "no migrations should be applied")
	assert.False(t, tableExists(t, session, "auth_migrations"), "no migrations should be applied")
	assert.False(t, tableExists(t, session, "tss_migrations"), "no migrations should be applied")
}

func TestClonesAreIsolated(t *testing.T) {
	db1 := Open(t)
	defer db1.Close()
	db2 := Open(t)
	defer db2.Close()

	session1 := db1.Open()
	defer session1.Close()
	session2 := db2.Open()
	defer session2.Close()

	_, err := session1.Exec(`CREATE TABLE isolation_check (id INT)`)
	require.NoError(t, err)

	assert.True(t, tableExists(t, session1, "isolation_check"), "table should exist in the clone it was created in")
	assert.False(t, tableExists(t, session2, "isolation_check"), "table must not leak into a sibling clone")
}

// Reports whether a table is present in the connected database.
func tableExists(t *testing.T, session *sqlx.DB, table string) bool {
	t.Helper()
	var exists bool
	err := session.Get(&exists, "SELECT to_regclass($1) IS NOT NULL", table)
	require.NoError(t, err)
	return exists
}
