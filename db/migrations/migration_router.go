package migrations

import (
	"net/http"

	adminmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/admin-migrations"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	tssmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/tss-migrations"
)

type MigrationRouter struct {
	TableName string
	FS        http.FileSystem
}

var (
	AdminMigrationRouter = MigrationRouter{TableName: "admin_migrations", FS: http.FS(adminmigrations.FS)}
	SDPMigrationRouter   = MigrationRouter{TableName: "sdp_migrations", FS: http.FS(sdpmigrations.FS)}
	AuthMigrationRouter  = MigrationRouter{TableName: "auth_migrations", FS: http.FS(authmigrations.FS)}
	TSSMigrationRouter   = MigrationRouter{TableName: "tss_migrations", FS: http.FS(tssmigrations.FS)}
)
