package cmd

import (
	"context"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	serveadmin "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type DistributionAccountCommand struct {
	CrashTrackerClient    crashtracker.CrashTrackerClient
	TSSDBConnectionPool   db.DBConnectionPool
	DistAccResolver       signing.DistributionAccountResolver
	AdminDBConnectionPool db.DBConnectionPool
}

func (c *DistributionAccountCommand) Command() *cobra.Command {
	crashTrackerOptions := crashtracker.CrashTrackerOptions{}
	distAccResolverOpts := signing.DistributionAccountResolverOptions{}
	configOpts := config.ConfigOptions{
		cmdUtils.CrashTrackerTypeConfigOption(&crashTrackerOptions.CrashTrackerType),
		cmdUtils.DistributionPublicKey(&distAccResolverOpts.HostDistributionAccountPublicKey),
	}

	distributionAccountCmd := &cobra.Command{
		Use:   "distribution-account",
		Short: "Distribution account related commands",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %v", err)
			}

			// Setup the TSSDBConnectionPool
			dbcpOptions := di.DBConnectionPoolOptions{DatabaseURL: globalOptions.DatabaseURL}
			c.TSSDBConnectionPool, err = di.NewTSSDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating TSS DB connection pool: %v", err)
			}

			// Initializing the AdminDBConnectionPool
			c.AdminDBConnectionPool, err = di.NewAdminDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error getting Admin DB connection pool: %v", err)
			}
			distAccResolverOpts.AdminDBConnectionPool = c.AdminDBConnectionPool

			// Initializing the DistributionAccountResolver
			distributionAccountResolver, err := di.NewDistributionAccountResolver(ctx, distAccResolverOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating distribution account resolver: %v", err)
			}
			c.DistAccResolver = distributionAccountResolver

			// Inject crash tracker options dependencies
			globalOptions.PopulateCrashTrackerOptions(&crashTrackerOptions)
			c.CrashTrackerClient, err = di.NewCrashTracker(ctx, crashTrackerOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating crash tracker client: %v", err)
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			di.CleanupInstanceByValue(cmd.Context(), c.TSSDBConnectionPool)
			di.CleanupInstanceByKey(cmd.Context(), di.AdminDBConnectionPoolInstanceName)
		},
	}
	err := configOpts.Init(distributionAccountCmd)
	if err != nil {
		log.Ctx(distributionAccountCmd.Context()).Fatalf("Error initializing %s command: %v", distributionAccountCmd.Name(), err)
	}

	distributionAccountCmd.AddCommand(c.RotateCommand())

	return distributionAccountCmd
}

func (c *DistributionAccountCommand) RotateCommand() *cobra.Command {
	var distAccService services.DistributionAccountServiceInterface
	var submitterEngine engine.SubmitterEngine
	var txSubmitterOpts di.TxSubmitterEngineOptions
	adminServeOpts := serveadmin.ServeOptions{}

	configOpts := cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)

	// Add tenant ID option
	var tenantID string
	configOpts = append(configOpts, &config.ConfigOption{
		Name:      "tenant-id",
		Usage:     "The ID of the tenant whose distribution account should be rotated",
		OptType:   types.String,
		ConfigKey: &tenantID,
		Required:  true,
	},
		cmdUtils.TenantXLMBootstrapAmount(&adminServeOpts.TenantAccountNativeAssetBootstrapAmount))

	rotateCmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate the distribution account for a tenant",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			if err := configOpts.SetValues(); err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options in %s: %v", cmd.Name(), err)
			}

			// Save Tenant to Context
			m := tenant.NewManager(tenant.WithDatabase(c.AdminDBConnectionPool))
			t, err := m.GetTenantByID(ctx, tenantID)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error getting tenant by ID: %v", err)
			}
			ctx = tenant.SaveTenantInContext(ctx, t)
			cmd.SetContext(ctx)

			// Prepare the signature service
			txSubmitterOpts.SignatureServiceOptions.DBConnectionPool = c.TSSDBConnectionPool
			txSubmitterOpts.SignatureServiceOptions.NetworkPassphrase = globalOptions.NetworkPassphrase
			txSubmitterOpts.SignatureServiceOptions.DistributionAccountResolver = c.DistAccResolver
			submitterEngine, err = di.NewTxSubmitterEngine(ctx, txSubmitterOpts)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating submitter engine: %v", err)
			}

			// Prepare the distribution account service
			distAccService, err = services.NewStellarDistributionAccountService(submitterEngine.HorizonClient)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating distribution account service: %v", err)
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()
			cmdService := DistAccCmdService{
				distAccService:             distAccService,
				submitterEngine:            submitterEngine,
				tenantManager:              tenant.NewManager(tenant.WithDatabase(c.AdminDBConnectionPool)),
				nativeAssetBootstrapAmount: adminServeOpts.TenantAccountNativeAssetBootstrapAmount,
				maxBaseFee:                 int64(txSubmitterOpts.MaxBaseFee),
			}
			err := cmdService.RotateDistributionAccount(ctx)
			if err != nil {
				c.CrashTrackerClient.LogAndReportErrors(ctx, err, "Cmd distribution-account rotate crash")
				log.Ctx(ctx).Fatalf("Error rotating distribution account: %v", err)
			}
		},
	}
	if err := configOpts.Init(rotateCmd); err != nil {
		log.Ctx(rotateCmd.Context()).Fatalf("Error initializing %s command: %v", rotateCmd.Name(), err)
	}

	return rotateCmd
}
