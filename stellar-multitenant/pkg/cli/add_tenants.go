package cli

import (
	"context"
	"fmt"
	"go/types"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"golang.org/x/exp/slices"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/cli/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var validTenantName *regexp.Regexp = regexp.MustCompile(`^[a-z-]+$`)

type AddTenantsCommandOptions struct {
	SDPUIBaseURL                            *string
	NetworkType                             string
	MessengerOptions                        message.MessengerOptions
	TenantAccountNativeAssetBootstrapAmount int
}

func AddTenantsCmd() *cobra.Command {
	tenantsOpts := AddTenantsCommandOptions{}
	configOptions := config.ConfigOptions{
		{
			Name:           "network-type",
			Usage:          "The Stellar Network type",
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionNetworkType,
			ConfigKey:      &tenantsOpts.NetworkType,
			FlagDefault:    "testnet",
			Required:       true,
		},
		{
			Name:           "sdp-ui-base-url",
			Usage:          "The Tenant SDP UI/dashboard Base URL.",
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionURLString,
			ConfigKey:      &tenantsOpts.SDPUIBaseURL,
			FlagDefault:    "http://localhost:3000",
			Required:       true,
		},
		{
			Name:           "email-sender-type",
			Usage:          fmt.Sprintf("The messenger type used to send invitations to new dashboard users. Options: %+v", message.MessengerType("").ValidEmailTypes()),
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionMessengerType,
			ConfigKey:      &tenantsOpts.MessengerOptions.MessengerType,
			Required:       true,
		},
		{
			Name:      "aws-access-key-id",
			Usage:     "The AWS access key ID",
			OptType:   types.String,
			ConfigKey: &tenantsOpts.MessengerOptions.AWSAccessKeyID,
			Required:  false,
		},
		{
			Name:      "aws-secret-access-key",
			Usage:     "The AWS secret access key",
			OptType:   types.String,
			ConfigKey: &tenantsOpts.MessengerOptions.AWSSecretAccessKey,
			Required:  false,
		},
		{
			Name:      "aws-region",
			Usage:     "The AWS region",
			OptType:   types.String,
			ConfigKey: &tenantsOpts.MessengerOptions.AWSRegion,
			Required:  false,
		},
		cmdUtils.TenantAccountNativeAssetBootstrapAmount(&tenantsOpts.TenantAccountNativeAssetBootstrapAmount),
	}

	txSubOpts := di.TxSubmitterEngineOptions{}
	configOptions = append(configOptions, cmdUtils.TransactionSubmitterEngineConfigOptions(&txSubOpts)...)
	configOptions = append(configOptions, cmdUtils.NetworkPassphrase(&txSubOpts.SignatureServiceOptions.NetworkPassphrase))

	distAccResolverOpts := signing.DistributionAccountResolverOptions{}
	configOptions = append(configOptions, cmdUtils.DistributionPublicKey(&distAccResolverOpts.HostDistributionAccountPublicKey))

	cmd := cobra.Command{
		Use:     "add-tenants",
		Short:   "Add a new tenant",
		Example: "add-tenants [tenant name] [user first name] [user last name] [user email] [organization name]",
		Long:    "Add a new tenant. The tenant name should only contain lower case characters and dash (-)",
		Args: cobra.MatchAll(
			cobra.ExactArgs(5),
			validateTenantNameArg,
		),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmdUtils.PropagatePersistentPreRun(cmd, args)
			configOptions.Require()
			err := configOptions.SetValues()
			if err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			// Get messenger client
			emailMessengerClient, err := di.NewEmailClient(di.EmailClientOptions{
				EmailType:        tenantsOpts.MessengerOptions.MessengerType,
				MessengerOptions: &tenantsOpts.MessengerOptions,
			})
			if err != nil {
				log.Ctx(ctx).Fatalf("creating email client: %v", err)
			}

			// Get TSS DB connection pool
			// TODO: in SDP-874, make sure to add metrics to this DB options, like we do in cmd/serve.go
			dbcpOptions := di.DBConnectionPoolOptions{DatabaseURL: globalOptions.multitenantDbURL}
			tssDBConnectionPool, err := di.NewTSSDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("getting TSS DBConnectionPool: %v", err)
			}
			defer func() {
				di.CleanupInstanceByValue(ctx, tssDBConnectionPool)
			}()

			// Get Admin DB connection pool
			adminDBConnectionPool, err := di.NewAdminDBConnectionPool(ctx, dbcpOptions)
			if err != nil {
				log.Ctx(ctx).Fatalf("getting Admin database connection pool: %v", err)
			}
			defer func() {
				di.CleanupInstanceByValue(ctx, adminDBConnectionPool)
			}()

			tenantName, userFirstName, userLastName, userEmail, organizationName := args[0], args[1], args[2], args[3], args[4]
			err = executeAddTenant(
				ctx,
				adminDBConnectionPool, tssDBConnectionPool,
				tenantName, userFirstName, userLastName, userEmail, organizationName,
				emailMessengerClient,
				tenantsOpts,
				txSubOpts,
				distAccResolverOpts,
			)
			if err != nil {
				log.Ctx(ctx).Fatalf("Error adding tenant: %v", err)
			}
		},
	}

	if err := configOptions.Init(&cmd); err != nil {
		log.Ctx(cmd.Context()).Fatalf("initializing config options: %v", err)
	}

	return &cmd
}

func validateTenantNameArg(cmd *cobra.Command, args []string) error {
	if !validTenantName.MatchString(args[0]) {
		return fmt.Errorf("invalid tenant name %q. It should only contains lower case letters and dash (-)", args[0])
	}
	return nil
}

func executeAddTenant(
	ctx context.Context,
	adminDBConnectionPool, tssDBConnectionPool db.DBConnectionPool,
	tenantName, userFirstName, userLastName, userEmail, organizationName string,
	messengerClient message.MessengerClient,
	tenantsOpts AddTenantsCommandOptions,
	txSubOpts di.TxSubmitterEngineOptions,
	distAccResolverOpts signing.DistributionAccountResolverOptions,
) error {
	if !slices.Contains(signing.DistributionSignatureClientTypes(), txSubOpts.SignatureServiceOptions.DistributionSignerType) {
		return fmt.Errorf("invalid distribution account signer type %q", txSubOpts.SignatureServiceOptions.DistributionSignerType)
	}
	txSubOpts.SignatureServiceOptions.DBConnectionPool = tssDBConnectionPool

	horizonClient := &horizonclient.Client{
		HorizonURL: txSubOpts.HorizonURL,
		HTTP:       httpclient.DefaultClient(),
	}

	distAccResolver, err := signing.NewDistributionAccountResolver(signing.DistributionAccountResolverOptions{
		AdminDBConnectionPool:            adminDBConnectionPool,
		HostDistributionAccountPublicKey: distAccResolverOpts.HostDistributionAccountPublicKey,
	})
	if err != nil {
		return fmt.Errorf("creating a new distribution account resolver instance: %w", err)
	}
	txSubOpts.SignatureServiceOptions.DistributionAccountResolver = distAccResolver

	ledgerNumberTracker, err := preconditions.NewLedgerNumberTracker(horizonClient)
	if err != nil {
		return fmt.Errorf("grabbing ledger number tracker instance: %w", err)
	}
	txSubOpts.SignatureServiceOptions.LedgerNumberTracker = ledgerNumberTracker

	signatureService, err := signing.NewSignatureService(txSubOpts.SignatureServiceOptions)
	if err != nil {
		return fmt.Errorf("grabbing signature service instance: %w", err)
	}

	txSubmitterEngine := engine.SubmitterEngine{
		HorizonClient:       horizonClient,
		LedgerNumberTracker: ledgerNumberTracker,
		SignatureService:    signatureService,
		MaxBaseFee:          txSubOpts.MaxBaseFee,
	}

	p := provisioning.NewManager(
		provisioning.WithDatabase(adminDBConnectionPool),
		provisioning.WithMessengerClient(messengerClient),
		provisioning.WithTenantManager(tenant.NewManager(tenant.WithDatabase(adminDBConnectionPool))),
		provisioning.WithSubmitterEngine(txSubmitterEngine),
		provisioning.WithNativeAssetBootstrapAmount(tenantsOpts.TenantAccountNativeAssetBootstrapAmount),
	)

	t, err := p.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, organizationName, *tenantsOpts.SDPUIBaseURL, tenantsOpts.NetworkType)
	if err != nil {
		return fmt.Errorf("adding tenant with name %s: %w", tenantName, err)
	}

	log.Ctx(ctx).Infof("tenant %s added successfully", tenantName)
	log.Ctx(ctx).Infof("tenant ID: %s", t.ID)

	return nil
}
