package cli

import (
	"context"
	"fmt"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var validTenantName *regexp.Regexp = regexp.MustCompile(`^[a-z-]+$`)

func AddTenantsCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:     "add-tenants",
		Short:   "Add a new tenant.",
		Example: "add-tenants [name]",
		Long:    "Add a new tenant. The tenant name should only contain lower case characters and dash (-)",
		Args: cobra.MatchAll(
			cobra.ExactArgs(1),
			validateTenantNameArg,
		),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := executeAddTenant(ctx, globalOptions.multitenantDbURL, args[0]); err != nil {
				log.Fatal(err)
			}
		},
	}

	return &cmd
}

func validateTenantNameArg(cmd *cobra.Command, args []string) error {
	if !validTenantName.MatchString(args[0]) {
		return fmt.Errorf("invalid tenant name %q. It should only contains lower case letters and dash (-)", args[0])
	}
	return nil
}

func executeAddTenant(ctx context.Context, dbURL, name string) error {
	dbConnectionPool, err := db.OpenDBConnectionPool(dbURL)
	if err != nil {
		return fmt.Errorf("opening database connection pool: %w", err)
	}
	defer dbConnectionPool.Close()

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	t, err := m.AddTenant(ctx, name)
	if err != nil {
		return fmt.Errorf("adding tenant with name %s: %w", name, err)
	}

	log.Ctx(ctx).Infof("tenant %s added successfully", name)
	log.Ctx(ctx).Infof("tenant ID: %s", t.ID)

	return nil
}
