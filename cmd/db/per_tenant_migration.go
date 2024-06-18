package db

import (
	"context"
	"fmt"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/migrations"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type TenantMigrationDetails struct {
	DSN                     string
	DistributionAccountType schema.AccountType
}

// executeMigrationsPerTenant executes the migrations on the database for all tenants or a specific tenant, according
// with the direction and count.
func executeMigrationsPerTenant(
	ctx context.Context,
	adminDBConnectionPool db.DBConnectionPool,
	opts utils.TenantRoutingOptions,
	dir migrate.MigrationDirection,
	count int,
	migrationRouter migrations.MigrationRouter,
) error {
	if err := opts.ValidateFlags(); err != nil {
		log.Ctx(ctx).Fatal(err.Error())
	}

	tntMigrationDetails, err := gatherTenantMigrationDetails(ctx, adminDBConnectionPool)
	if err != nil {
		return fmt.Errorf("getting tenants schemas: %w", err)
	}

	if opts.TenantID != "" {
		if migrationDetails, ok := tntMigrationDetails[opts.TenantID]; ok {
			tntMigrationDetails = map[string]TenantMigrationDetails{
				opts.TenantID: migrationDetails,
			}
		} else {
			return fmt.Errorf("tenant ID %s does not exist", opts.TenantID)
		}
	}

	for tenantID, tntMigrationDetails := range tntMigrationDetails {
		log.Ctx(ctx).Infof("Applying migrations on tenant ID %s", tenantID)
		err = ExecuteMigrations(ctx, tntMigrationDetails.DSN, dir, count, migrationRouter)
		if err != nil {
			return fmt.Errorf("migrating database %s: %w", migrationDirectionStr(dir), err)
		}
	}

	return nil
}

// gatherTenantMigrationDetails returns a map of tenant IDs to their respective migration details (database DSN and DistributionAccountType).
func gatherTenantMigrationDetails(ctx context.Context, adminDBConnectionPool db.DBConnectionPool) (map[string]TenantMigrationDetails, error) {
	m := tenant.NewManager(tenant.WithDatabase(adminDBConnectionPool))
	tenants, err := m.GetAllTenants(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("getting all tenants: %w", err)
	}

	migrationDetailsMap := make(map[string]TenantMigrationDetails, len(tenants))
	for _, tnt := range tenants {
		dsn, err := m.GetDSNForTenant(ctx, tnt.Name)
		if err != nil {
			return nil, fmt.Errorf("getting DSN for tenant %s: %w", tnt.Name, err)
		}
		migrationDetailsMap[tnt.ID] = TenantMigrationDetails{
			DSN:                     dsn,
			DistributionAccountType: tnt.DistributionAccountType,
		}
	}

	return migrationDetailsMap, nil
}
