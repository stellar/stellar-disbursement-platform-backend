package cli

import (
	"context"
	"fmt"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/cli/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type tenantOptions struct {
	ID              string
	EmailSenderType *tenant.EmailSenderType
	SMSSenderType   *tenant.SMSSenderType
	BaseURL         *string
	SDPUIBaseURL    *string
}

func ConfigTenantCmd() *cobra.Command {
	to := tenantOptions{}
	configOptions := config.ConfigOptions{
		{
			Name:      "tenant-id",
			Usage:     "The ID of the Tenant to configure.",
			OptType:   types.String,
			ConfigKey: &to.ID,
			Required:  true,
		},
		{
			Name:           "email-sender-type",
			Usage:          `The messenger type used to send invitations to new dashboard users. Options: "AWS_EMAIL", "DRY_RUN"`,
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionEmailSenderType,
			ConfigKey:      &to.EmailSenderType,
			Required:       false,
		},
		{
			Name:           "sms-sender-type",
			Usage:          `SMS Sender Type. Options: "TWILIO_SMS", "AWS_SMS", "DRY_RUN"`,
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionSMSSenderType,
			ConfigKey:      &to.SMSSenderType,
			Required:       false,
		},
		{
			Name:           "base-url",
			Usage:          "The SDP backend server's base URL.",
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionURLString,
			ConfigKey:      &to.BaseURL,
			Required:       false,
		},
		{
			Name:           "sdp-ui-base-url",
			Usage:          "The SDP UI/dashboard Base URL.",
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionURLString,
			ConfigKey:      &to.SDPUIBaseURL,
			Required:       false,
		},
	}

	var adminDBConnectionPool db.DBConnectionPool
	cmd := cobra.Command{
		Use:     "config-tenant",
		Short:   "Configure an existing tenant",
		Long:    "Configure an existing tenant by updating their existing configuration",
		Aliases: []string{"ct"},
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

			var err error
			// TODO: in SDP-874, make sure to add metrics to this DB options, like we do in cmd/serve.go
			adminDBConnectionPool, err = di.NewAdminDBConnectionPool(ctx, di.DBConnectionPoolOptions{DatabaseURL: globalOptions.multitenantDbURL})
			if err != nil {
				log.Ctx(ctx).Fatal("getting admin db connection pool", err)
			}

			if err := executeConfigTenant(ctx, &to, adminDBConnectionPool); err != nil {
				log.Ctx(ctx).Fatal(err)
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			di.DeleteAndCloseInstanceByValue(cmd.Context(), adminDBConnectionPool)
		},
	}

	if err := configOptions.Init(&cmd); err != nil {
		log.Ctx(cmd.Context()).Fatalf("initializing ConfigTenantCmd config options: %v", err)
	}

	return &cmd
}

func executeConfigTenant(ctx context.Context, to *tenantOptions, dbConnectionPool db.DBConnectionPool) error {
	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	_, err := m.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
		ID:              to.ID,
		EmailSenderType: to.EmailSenderType,
		SMSSenderType:   to.SMSSenderType,
		BaseURL:         to.BaseURL,
		SDPUIBaseURL:    to.SDPUIBaseURL,
	})
	if err != nil {
		return fmt.Errorf("updating tenant config: %w", err)
	}

	log.Infof("tenant ID %s configuration updated successfully", to.ID)

	return nil
}
