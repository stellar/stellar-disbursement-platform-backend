package migrations

import (
	"io/fs"

	adminmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/admin-migrations"
	authmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/auth-migrations"
	sdpmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/sdp-migrations"
	tssmigrations "github.com/stellar/stellar-disbursement-platform-backend/db/migrations/tss-migrations"
)

type MigrationRouter struct {
	TableName string
	FS        fs.FS
}

var (
	AdminMigrationRouter = MigrationRouter{TableName: "admin_migrations", FS: fs.FS(adminmigrations.FS)}
	SDPMigrationRouter   = MigrationRouter{TableName: "sdp_migrations", FS: fs.FS(sdpmigrations.FS)}
	AuthMigrationRouter  = MigrationRouter{TableName: "auth_migrations", FS: fs.FS(authmigrations.FS)}
	TSSMigrationRouter   = MigrationRouter{TableName: "tss_migrations", FS: fs.FS(tssmigrations.FS)}
)
