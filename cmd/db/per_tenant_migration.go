package db

import (
	"context"
	"embed"
	"fmt"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// executeMigrationsPerTenant executes the migrations on the database for all tenants or a specific tenant, according
// with the direction and count.
func executeMigrationsPerTenant(ctx context.Context, databaseURL string, opts utils.TenantRoutingOptions, dir migrate.MigrationDirection, count int, migrationFiles embed.FS, tableName db.MigrationTableName) error {
	if err := opts.ValidateFlags(); err != nil {
		log.Ctx(ctx).Fatal(err.Error())
	}

	tenantIDToDNSMap, err := getTenantIDToDSNMapping(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("getting tenants schemas: %w", err)
	}

	if opts.TenantID != "" {
		if dsn, ok := tenantIDToDNSMap[opts.TenantID]; ok {
			tenantIDToDNSMap = map[string]string{opts.TenantID: dsn}
		} else {
			log.Ctx(ctx).Fatalf("tenant ID %s does not exist", opts.TenantID)
		}
	}

	for tenantID, dsn := range tenantIDToDNSMap {
		log.Ctx(ctx).Infof("Applying migrations on tenant ID %s", tenantID)
		err = ExecuteMigrations(ctx, dsn, dir, count, migrationFiles, tableName)
		if err != nil {
			log.Ctx(ctx).Fatalf("Error migrating database Up: %s", err.Error())
		}
	}

	return nil
}

// getTenantIDToDSNMapping returns a map of tenant IDs to their Database's DSN.
func getTenantIDToDSNMapping(ctx context.Context, dbURL string) (map[string]string, error) {
	dbConnectionPool, err := db.OpenDBConnectionPool(dbURL)
	if err != nil {
		return nil, fmt.Errorf("opening database connection pool: %w", err)
	}
	defer dbConnectionPool.Close()

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	tenants, err := m.GetAllTenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting all tenants: %w", err)
	}

	tenantsDSNMap := make(map[string]string, len(tenants))
	for _, tnt := range tenants {
		dsn, err := m.GetDSNForTenant(ctx, tnt.Name)
		if err != nil {
			return nil, fmt.Errorf("getting DSN for tenant %s: %w", tnt.Name, err)
		}
		tenantsDSNMap[tnt.ID] = dsn
	}

	return tenantsDSNMap, nil
}
