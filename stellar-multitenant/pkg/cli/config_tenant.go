package cli

import (
	"context"
	"fmt"
	"go/types"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/cli/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type tenantOptions struct {
	ID                 string
	EmailSenderType    *tenant.EmailSenderType
	SMSSenderType      *tenant.SMSSenderType
	EnableMFA          *bool
	EnableReCAPTCHA    *bool
	CORSAllowedOrigins []string
	BaseURL            *string
	SDPUIBaseURL       *string
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
			Name:           "enable-mfa",
			Usage:          "Enable MFA using email.",
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionOptionalBoolean,
			ConfigKey:      &to.EnableMFA,
			Required:       false,
		},
		{
			Name:           "enable-recaptcha",
			Usage:          "Enable ReCAPTCHA for login and forgot password.",
			OptType:        types.String,
			CustomSetValue: utils.SetConfigOptionOptionalBoolean,
			ConfigKey:      &to.EnableReCAPTCHA,
			Required:       false,
		},
		{
			Name:           "cors-allowed-origins",
			Usage:          `Cors URLs that are allowed to access the endpoints, separated by ","`,
			OptType:        types.String,
			CustomSetValue: utils.SetCORSAllowedOrigins,
			ConfigKey:      &to.CORSAllowedOrigins,
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

	cmd := cobra.Command{
		Use:     "config-tenant",
		Short:   "",
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
			if err := executeConfigTenant(ctx, &to, globalOptions.multitenantDbURL); err != nil {
				log.Fatal(err)
			}
		},
	}

	if err := configOptions.Init(&cmd); err != nil {
		log.Fatalf("initializing ConfigTenantCmd config options: %v", err)
	}

	return &cmd
}

func executeConfigTenant(ctx context.Context, to *tenantOptions, dbURL string) error {
	dbConnectionPool, err := db.OpenDBConnectionPool(dbURL)
	if err != nil {
		return fmt.Errorf("opening database connection pool: %w", err)
	}
	defer dbConnectionPool.Close()

	m := tenant.NewManager(tenant.WithDatabase(dbConnectionPool))
	_, err = m.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
		ID:                 to.ID,
		EmailSenderType:    to.EmailSenderType,
		SMSSenderType:      to.SMSSenderType,
		EnableMFA:          to.EnableMFA,
		EnableReCAPTCHA:    to.EnableReCAPTCHA,
		CORSAllowedOrigins: to.CORSAllowedOrigins,
		BaseURL:            to.BaseURL,
		SDPUIBaseURL:       to.SDPUIBaseURL,
	})
	if err != nil {
		return fmt.Errorf("updating tenant config: %w", err)
	}

	log.Infof("tenant ID %s configuration updated successfully", to.ID)

	return nil
}
