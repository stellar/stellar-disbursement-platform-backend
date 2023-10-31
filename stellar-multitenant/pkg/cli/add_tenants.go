package cli

import (
	"context"
	"fmt"
	"go/types"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/cli/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var validTenantName *regexp.Regexp = regexp.MustCompile(`^[a-z-]+$`)

func AddTenantsCmd() *cobra.Command {
	var networkType string
	configOptions := config.ConfigOptions{
		{
			Name:           "network-type",
			Usage:          "",
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionNetworkType,
			ConfigKey:      &networkType,
			FlagDefault:    "testnet",
			Required:       false,
		},
	}

	cmd := cobra.Command{
		Use:     "add-tenants",
		Short:   "Add a new tenant.",
		Example: "add-tenants [tenant name] [user first name] [user last name] [user email]",
		Long:    "Add a new tenant. The tenant name should only contain lower case characters and dash (-)",
		Args: cobra.MatchAll(
			cobra.ExactArgs(4),
			validateTenantNameArg,
		),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			configOptions.Require()
			err := configOptions.SetValues()
			if err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if err := executeAddTenant(ctx, globalOptions.multitenantDbURL, args[0], args[1], args[2], args[3], networkType); err != nil {
				log.Fatal(err)
			}
		},
	}

	if err := configOptions.Init(&cmd); err != nil {
		log.Fatalf("initializing config options: %v", err)
	}

	return &cmd
}

func validateTenantNameArg(cmd *cobra.Command, args []string) error {
	if !validTenantName.MatchString(args[0]) {
		return fmt.Errorf("invalid tenant name %q. It should only contains lower case letters and dash (-)", args[0])
	}
	return nil
}

func executeAddTenant(ctx context.Context, dbURL, tenantName, userFirstName, userLastName, userEmail, networkType string) error {
	dbConnectionPool, err := db.OpenDBConnectionPool(dbURL)
	if err != nil {
		return fmt.Errorf("opening database connection pool: %w", err)
	}
	defer dbConnectionPool.Close()

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	t, err := m.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, networkType)
	if err != nil {
		return fmt.Errorf("adding tenant with name %s: %w", tenantName, err)
	}

	log.Ctx(ctx).Infof("tenant %s added successfully", tenantName)
	log.Ctx(ctx).Infof("tenant ID: %s", t.ID)

	return nil
}
