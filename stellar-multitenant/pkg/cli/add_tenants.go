package cli

import (
	"context"
	"fmt"
	"go/types"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"golang.org/x/exp/slices"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/cli/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

var validTenantName *regexp.Regexp = regexp.MustCompile(`^[a-z-]+$`)

type AddTenantsCommandOptions struct {
	SDPUIBaseURL     *string
	NetworkType      string
	MessengerOptions message.MessengerOptions
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
	}

	sigOpts := signing.SignatureServiceOptions{}
	configOptions = append(configOptions,
		&config.ConfigOption{
			Name:        "network-passphrase",
			Usage:       "The Stellar Network passphrase",
			OptType:     types.String,
			FlagDefault: network.TestNetworkPassphrase,
			ConfigKey:   &sigOpts.NetworkPassphrase,
			Required:    true,
		})
	configOptions = append(configOptions, cmdUtils.BaseDistributionAccountSignatureClientConfigOptions(&sigOpts)...)

	cmd := cobra.Command{
		Use:     "add-tenants",
		Short:   "Add a new tenant.",
		Example: "add-tenants [tenant name] [user first name] [user last name] [user email] [organization name]",
		Long:    "Add a new tenant. The tenant name should only contain lower case characters and dash (-)",
		Args: cobra.MatchAll(
			cobra.ExactArgs(5),
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
			messengerClient, err := message.GetClient(tenantsOpts.MessengerOptions)
			if err != nil {
				log.Fatalf("creating email client: %v", err)
			}

			ctx := cmd.Context()
			if err := executeAddTenant(
				ctx, globalOptions.multitenantDbURL, args[0], args[1], args[2], args[3], args[4],
				messengerClient, tenantsOpts, sigOpts); err != nil {
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

func executeAddTenant(
	ctx context.Context, dbURL, tenantName, userFirstName, userLastName, userEmail,
	organizationName string, messengerClient message.MessengerClient, tenantsOpts AddTenantsCommandOptions, sigOpts signing.SignatureServiceOptions,
) error {
	dbConnectionPool, err := db.OpenDBConnectionPool(dbURL)
	if err != nil {
		return fmt.Errorf("opening database connection pool: %w", err)
	}
	defer dbConnectionPool.Close()

	if !slices.Contains(signing.DistributionSignatureClientTypes(), sigOpts.DistributionSignerType) {
		return fmt.Errorf("invalid distribution account signer type %q", sigOpts.DistributionSignerType)
	}

	// We need to use a dbConnectionPool that resolves to the tss namespace for the distribution account signature client.
	tssDBConnectionPool, err := dependencyinjection.NewTSSDBConnectionPool(ctx, dependencyinjection.TSSDBConnectionPoolOptions{DatabaseURL: dbURL})
	if err != nil {
		return fmt.Errorf("getting TSS DBConnectionPool: %w", err)
	}
	defer tssDBConnectionPool.Close()

	distAccSigClient, err := signing.NewSignatureClient(sigOpts.DistributionSignerType, signing.SignatureClientOptions{
		NetworkPassphrase:           sigOpts.NetworkPassphrase,
		DistributionPrivateKey:      sigOpts.DistributionPrivateKey,
		DistAccEncryptionPassphrase: sigOpts.DistAccEncryptionPassphrase,
		DBConnectionPool:            tssDBConnectionPool,
	})
	if err != nil {
		return fmt.Errorf("creating a new distribution account signature client: %w", err)
	}

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	p := provisioning.NewManager(
		provisioning.WithDatabase(dbConnectionPool),
		provisioning.WithMessengerClient(messengerClient),
		provisioning.WithTenantManager(m),
		provisioning.WithDistributionAccountSignatureClient(distAccSigClient),
	)

	t, err := p.ProvisionNewTenant(ctx, tenantName, userFirstName, userLastName, userEmail, organizationName, *tenantsOpts.SDPUIBaseURL, tenantsOpts.NetworkType)
	if err != nil {
		return fmt.Errorf("adding tenant with name %s: %w", tenantName, err)
	}

	log.Ctx(ctx).Infof("tenant %s added successfully", tenantName)
	log.Ctx(ctx).Infof("tenant ID: %s", t.ID)

	return nil
}
