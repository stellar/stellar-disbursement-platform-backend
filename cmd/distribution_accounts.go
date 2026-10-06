package cmd

import (
	"context"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go-stellar-sdk/support/config"
	"github.com/stellar/go-stellar-sdk/support/log"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	tssSvc "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	serveadmin "github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/serve"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type DistributionAccountCommand struct {
	CrashTrackerClient    crashtracker.CrashTrackerClient
	TSSDBConnectionPool   db.DBConnectionPool
	DistAccResolver       signing.DistributionAccountResolver
	AdminDBConnectionPool db.DBConnectionPool
}

func (c *DistributionAccountCommand) Command(cmdService DistAccCmdServiceInterface) *cobra.Command {
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
			dbcpOptions := di.DBConnectionPoolOptions{
				DatabaseURL:            globalOptions.DatabaseURL,
				MaxOpenConns:           globalOptions.DBPool.DBMaxOpenConns,
				MaxIdleConns:           globalOptions.DBPool.DBMaxIdleConns,
				ConnMaxIdleTimeSeconds: globalOptions.DBPool.DBConnMaxIdleTimeSeconds,
				ConnMaxLifetimeSeconds: globalOptions.DBPool.DBConnMaxLifetimeSeconds,
			}
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

	distributionAccountCmd.AddCommand(c.RotateCommand(cmdService))
	distributionAccountCmd.AddCommand(c.RotateWalletDEKCommand())

	return distributionAccountCmd
}

func (c *DistributionAccountCommand) RotateCommand(cmdService DistAccCmdServiceInterface) *cobra.Command {
	var distAccService services.DistributionAccountServiceInterface
	var submitterEngine engine.SubmitterEngine
	var txSubmitterOpts di.TxSubmitterEngineOptions
	var tenantRoutingOpts cmdUtils.TenantRoutingOptions
	var walletID string
	adminServeOpts := serveadmin.ServeOptions{}

	configOpts := cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubmitterOpts)
	configOpts = append(configOpts,
		cmdUtils.SingleTenantRoutingConfigOptions(&tenantRoutingOpts),
		cmdUtils.TenantXLMBootstrapAmount(&adminServeOpts.TenantAccountNativeAssetBootstrapAmount),
		&config.ConfigOption{
			Name:      "wallet-id",
			Usage:     "ID of the distribution wallet to rotate. When omitted, the tenant's default wallet (its distribution account) is rotated.",
			OptType:   types.String,
			ConfigKey: &walletID,
			Required:  false,
		})

	rotateCmd := &cobra.Command{
		Use:   "rotate",
		Short: "Rotate the distribution account of a tenant's distribution wallet",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			if err := configOpts.SetValues(); err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options in %s: %v", cmd.Name(), err)
			}

			// Save Tenant to Context
			if tenantRoutingOpts.TenantID == "" {
				log.Ctx(ctx).Fatalf("Tenant ID is required")
			}
			m := tenant.NewManager(tenant.WithDatabase(c.AdminDBConnectionPool))
			t, err := m.GetTenantByID(ctx, tenantRoutingOpts.TenantID)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error getting tenant by ID: %v", err)
			}
			ctx = sdpcontext.SetTenantInContext(ctx, t)
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
			tenantManager := tenant.NewManager(tenant.WithDatabase(c.AdminDBConnectionPool))

			// Rotation writes the tenant schema's distribution_wallets row, which the admin pool
			// cannot reach, so open a pool scoped to the tenant schema for the duration of the run.
			t, err := sdpcontext.GetTenantFromContext(ctx)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error getting tenant from context: %v", err)
			}
			tenantSchemaDSN, err := tenantManager.GetDSNForTenant(ctx, t.Name)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error getting DSN for tenant %s: %v", t.Name, err)
			}
			tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(tenantSchemaDSN)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error opening database connection on tenant schema: %v", err)
			}
			defer utils.DeferredClose(ctx, tenantSchemaConnectionPool, "closing tenant schema connection pool after rotating the distribution account")

			models, err := data.NewModels(tenantSchemaConnectionPool)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error getting models for tenant schema: %v", err)
			}

			// Per-wallet key material lives in the TSS database, envelope-encrypted under the same
			// host KEK the signature service uses.
			walletKeyService, err := tssSvc.NewDistributionWalletKeyService(c.TSSDBConnectionPool, txSubmitterOpts.SignatureServiceOptions.DistAccEncryptionPassphrase)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error creating distribution wallet key service: %v", err)
			}

			distributionAccService := DistributionAccountService{
				distAccService:             distAccService,
				submitterEngine:            submitterEngine,
				tenantManager:              tenantManager,
				nativeAssetBootstrapAmount: adminServeOpts.TenantAccountNativeAssetBootstrapAmount,
				maxBaseFee:                 int64(txSubmitterOpts.MaxBaseFee),
				models:                     models,
				walletKeyService:           walletKeyService,
				walletID:                   walletID,
			}
			if err = cmdService.RotateDistributionAccount(ctx, distributionAccService); err != nil {
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
