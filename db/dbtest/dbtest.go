package dbtest

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go-stellar-sdk/support/db/dbtest"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
)

var (
	templatesMu sync.Mutex
	templates   = map[string]string{} // migration set key -> template db name
)

func OpenWithoutMigrations(t *testing.T) *dbtest.DB {
	t.Helper()

	db := dbtest.Postgres(t)
	return db
}

func openWithMigrations(t *testing.T, configs ...migrations.MigrationRouter) *dbtest.DB {
	t.Helper()

	db := dbtest.Postgres(t)

	tmpl := ensureTemplate(t, db.DSN, configs)
	createFromTemplate(t, db, tmpl)
	return db
}

func Open(t *testing.T) *dbtest.DB {
	t.Helper()

	return openWithMigrations(t,
		migrations.AdminMigrationRouter,
		migrations.SDPMigrationRouter,
		migrations.AuthMigrationRouter,
		migrations.TSSMigrationRouter,
	)
}

func OpenWithAdminMigrationsOnly(t *testing.T) *dbtest.DB {
	t.Helper()

	return openWithMigrations(t, migrations.AdminMigrationRouter)
}

func OpenWithTSSMigrationsOnly(t *testing.T) *dbtest.DB {
	t.Helper()

	return openWithMigrations(t, migrations.TSSMigrationRouter)
}

// Builds and caches one migrated template DB per migration set (once per process)
func ensureTemplate(t *testing.T, refDSN string, configs []migrations.MigrationRouter) string {
	t.Helper()

	key := routerKey(configs)

	templatesMu.Lock()
	defer templatesMu.Unlock()
	if name, ok := templates[key]; ok {
		return name
	}

	// PID keeps templates unique across parallel test binaries; hash separates migration set variants within a process.
	name := fmt.Sprintf("sdp_tmpl_%d_%s", os.Getpid(), key)

	admin := adminConn(t, refDSN)
	defer admin.Close()

	mustExec(t, admin, "DROP DATABASE IF EXISTS "+pq.QuoteIdentifier(name))
	mustExec(t, admin, "CREATE DATABASE "+pq.QuoteIdentifier(name))

	pool, err := sql.Open("postgres", withDBName(t, refDSN, name))
	require.NoError(t, err, "opening template pool")
	for _, config := range configs {
		ms := migrate.MigrationSet{TableName: config.TableName}
		src := migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(config.FS)}
		if _, err := ms.ExecMax(pool, "postgres", src, migrate.Up, 0); err != nil {
			_ = pool.Close()
			require.NoError(t, err, "migrating template")
		}
	}
	require.NoError(t, pool.Close(), "closing template pool")

	mustExec(t, admin, fmt.Sprintf(
		"ALTER DATABASE %s WITH ALLOW_CONNECTIONS false IS_TEMPLATE true",
		pq.QuoteIdentifier(name),
	))

	templates[key] = name
	return name
}

// Re-creates the empty database as a fast clone of the template.
func createFromTemplate(t *testing.T, db *dbtest.DB, tmpl string) {
	t.Helper()

	target := dbName(t, db.DSN)

	admin := adminConn(t, db.DSN)
	defer admin.Close()

	mustExec(t, admin, "DROP DATABASE IF EXISTS "+pq.QuoteIdentifier(target))
	mustExec(t, admin, fmt.Sprintf(
		"CREATE DATABASE %s TEMPLATE %s",
		pq.QuoteIdentifier(target), pq.QuoteIdentifier(tmpl),
	))
}

func routerKey(configs []migrations.MigrationRouter) string {
	h := fnv.New32a()
	for _, c := range configs {
		h.Write([]byte(c.TableName))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%08x", h.Sum32())
}

func adminConn(t *testing.T, refDSN string) *sql.DB {
	t.Helper()
	conn, err := sql.Open("postgres", withDBName(t, refDSN, "postgres"))
	require.NoError(t, err, "opening admin conn")
	return conn
}

func withDBName(t *testing.T, dsn, name string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	require.NoError(t, err, "parsing dsn")
	u.Path = "/" + name
	return u.String()
}

func dbName(t *testing.T, dsn string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	require.NoError(t, err, "parsing dsn")
	return strings.TrimPrefix(u.Path, "/")
}

func mustExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	_, err := db.Exec(query)
	require.NoErrorf(t, err, "exec %q", query)
}
