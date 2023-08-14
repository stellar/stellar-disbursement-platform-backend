package cmd

import (
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/integrationtests"
)

type IntegrationTestsCommand struct {
	Service integrationtests.IntegrationTestsInterface
}

func (c *IntegrationTestsCommand) Command() *cobra.Command {
	integrationTestsOpts := &integrationtests.IntegrationTestsOpts{}

	configOpts := config.ConfigOptions{
		{
			Name:      "disbursed-asset-code",
			Usage:     "Code of the asset to be disbursed",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.DisbursedAssetCode,
			Required:  true,
		},
		{
			Name:      "disbursed-asset-issuer",
			Usage:     "Issuer if the asset to be disbursed",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.DisbursetAssetIssuer,
			Required:  true,
		},
		{
			Name:        "disbursement-name",
			Usage:       "Disbursement name to be used in integration tests",
			OptType:     types.String,
			ConfigKey:   &integrationTestsOpts.DisbursementName,
			FlagDefault: "disbursement_integration_tests",
			Required:    true,
		},
		{
			Name:        "wallet-name",
			Usage:       "Wallet name to be used in integration tests",
			OptType:     types.String,
			ConfigKey:   &integrationTestsOpts.WalletName,
			FlagDefault: "Integration test wallet",
			Required:    true,
		},
	}
	integrationTestsCmd := &cobra.Command{
		Use:   "integration-tests",
		Short: "Integration tests related commands",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Fatalf("Error setting values of config options: %s", err.Error())
			}

			// inject database url to integration tests opts
			integrationTestsOpts.DatabaseDSN = globalOptions.databaseURL

			c.Service, err = integrationtests.NewIntegrationTestsService(*integrationTestsOpts)
			if err != nil {
				log.Fatalf("error creating integration tests service: %s", err.Error())
			}
		},
	}
	err := configOpts.Init(integrationTestsCmd)
	if err != nil {
		log.Fatalf("Error initializing a config option: %s", err.Error())
	}

	startIntegrationTestsCmd := c.StartIntegrationTestsCommand(integrationTestsOpts)
	createIntegrationTestsDataCmd := c.CreateIntegrationTestsDataCommand(integrationTestsOpts)
	integrationTestsCmd.AddCommand(startIntegrationTestsCmd, createIntegrationTestsDataCmd)

	return integrationTestsCmd
}

func (c *IntegrationTestsCommand) StartIntegrationTestsCommand(integrationTestsOpts *integrationtests.IntegrationTestsOpts) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:      "user-email",
			Usage:     "Email from SDP authenticated user with all roles",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.UserEmail,
			Required:  true,
		},
		{
			Name:      "user-password",
			Usage:     "Password from SDP authenticated user with all roles",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.UserPassword,
			Required:  true,
		},
		{
			Name:           "receiver-account-public-key",
			Usage:          "Integration test receiver public stellar account key",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPublicKey,
			ConfigKey:      &integrationTestsOpts.ReceiverAccountPublicKey,
			Required:       true,
		},
		{
			Name:           "receiver-account-private-key",
			Usage:          "Integration test receiver private stellar account key",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPrivateKey,
			ConfigKey:      &integrationTestsOpts.ReceiverAccountPrivateKey,
			Required:       true,
		},
		{
			Name:      "receiver-account-stellar-memo",
			Usage:     "Integration test receiver stellar memo",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.ReceiverAccountStellarMemo,
			Required:  false,
		},
		{
			Name:           "sep10-signing-public-key",
			Usage:          "Anchor platform SEP10 signing public key",
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionStellarPublicKey,
			ConfigKey:      &integrationTestsOpts.Sep10SigningPublicKey,
			Required:       true,
		},
		{
			Name:      "disbursement-csv-file-name",
			Usage:     "File name of the integration test disbursement file.",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.DisbursementCSVFileName,
			Required:  true,
		},
		{
			Name:      "disbursement-csv-file-path",
			Usage:     "File path of the integration test disbursement file.",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.DisbursementCSVFilePath,
			Required:  true,
		},
		{
			Name:      "server-api-base-url",
			Usage:     "The Base URL of the server API of the SDP.",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.ServerApiBaseURL,
			Required:  true,
		},
		{
			Name: "anchor-platform-base-sep-url",
			Usage: "The Base URL of the sep server of the anchor platform. This is the base URL where the Anchor Platform " +
				"exposes its public API that is meant to be reached by a client application, such as the stellar.toml file.",
			OptType:   types.String,
			ConfigKey: &integrationTestsOpts.AnchorPlatformBaseSepURL,
			Required:  true,
		},
		{
			Name:        "recaptcha-site-key",
			Usage:       "The Google reCAPTCHA v2 - I'm not a robot site key.",
			OptType:     types.String,
			ConfigKey:   &integrationTestsOpts.RecaptchaSiteKey,
			FlagDefault: "6LeIxAcTAAAAAJcZVRqyHh71UMIEGNQ_MXjiZKhI",
			Required:    true,
		},
	}

	startIntegrationTestsCmd := &cobra.Command{
		Use:   "start",
		Short: "Run the e2e tests of the sdp application",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			err := c.Service.StartIntegrationTests(ctx, *integrationTestsOpts)
			if err != nil {
				log.Fatalf("Error starting integration tests: %s", err.Error())
			}
		},
	}

	err := configOpts.Init(startIntegrationTestsCmd)
	if err != nil {
		log.Fatalf("Error initializing startIntegrationTestsCmd: %s", err.Error())
	}

	return startIntegrationTestsCmd
}

func (c *IntegrationTestsCommand) CreateIntegrationTestsDataCommand(integrationTestsOpts *integrationtests.IntegrationTestsOpts) *cobra.Command {
	configOpts := config.ConfigOptions{
		{
			Name:        "wallet-homepage",
			Usage:       "Wallet homepage to be used in integration tests",
			OptType:     types.String,
			ConfigKey:   &integrationTestsOpts.WalletHomepage,
			FlagDefault: "https://www.test_wallet.com",
			Required:    true,
		},
		{
			Name:        "wallet-deeplink",
			Usage:       "Wallet deeplink to be used in integration tests",
			OptType:     types.String,
			ConfigKey:   &integrationTestsOpts.WalletDeepLink,
			FlagDefault: "test_wallet://",
			Required:    true,
		},
	}

	createIntegrationTestsDataCmd := &cobra.Command{
		Use:   "create-data",
		Short: "Create integration tests data.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			// Validate & ingest input parameters
			configOpts.Require()
			err := configOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("Error setting values of config options: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, _ []string) {
			ctx := cmd.Context()

			err := c.Service.CreateTestData(ctx, *integrationTestsOpts)
			if err != nil {
				log.Fatalf("Error creating integration tests data: %s", err.Error())
			}
		},
	}

	err := configOpts.Init(createIntegrationTestsDataCmd)
	if err != nil {
		log.Fatalf("Error initializing createIntegrationTestsDataCmd: %s", err.Error())
	}

	return createIntegrationTestsDataCmd
}
