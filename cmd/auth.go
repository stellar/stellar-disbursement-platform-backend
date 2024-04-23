package cmd

import (
	"fmt"
	"go/types"
	"net/url"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/router"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"

	cmdDB "github.com/stellar/stellar-disbursement-platform-backend/cmd/db"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/cli"
)

type AuthCommand struct{}

func (a *AuthCommand) Command() *cobra.Command {
	var uiBaseURL string
	messengerOptions := message.MessengerOptions{}

	authCmdConfigOpts := config.ConfigOptions{
		{
			Name:           "sdp-ui-base-url",
			Usage:          "The SDP UI/dashboard Base URL used to send the invitation link when a new user is created.",
			OptType:        types.String,
			ConfigKey:      &uiBaseURL,
			FlagDefault:    "http://localhost:3000",
			CustomSetValue: cmdUtils.SetConfigOptionURLString,
			Required:       true,
		},
		{
			Name:           "email-sender-type",
			Usage:          fmt.Sprintf("The messenger type used to send invitations to new dashboard users. Options: %+v", message.MessengerType("").ValidEmailTypes()),
			OptType:        types.String,
			CustomSetValue: cmdUtils.SetConfigOptionMessengerType,
			ConfigKey:      &messengerOptions.MessengerType,
			FlagDefault:    string(message.MessengerTypeDryRun),
			Required:       true,
		},
	}
	authCmdConfigOpts = append(authCmdConfigOpts, cmdUtils.TwilioConfigOptions(&messengerOptions)...)
	authCmdConfigOpts = append(authCmdConfigOpts, cmdUtils.AWSConfigOptions(&messengerOptions)...)

	var emailMessengerClient message.MessengerClient

	// Auth Module sub-commands
	availableRoles := data.FromUserRoleArrayToStringArray(data.GetAllRoles())
	addUserSubcommand := cli.AddUserCmd(cmdDB.DBConfigOptionFlagName, cli.NewDefaultPasswordPrompt(), availableRoles)

	authCmd := &cobra.Command{
		Use:     "auth",
		Short:   "Stellar Auth helpers",
		Example: "auth <sub-command>",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			cmd.Parent().PersistentPreRun(cmd.Parent(), args)

			authCmdConfigOpts.Require()
			err := authCmdConfigOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("error setting values of config options: %s", err.Error())
			}

			if cmd.Name() == addUserSubcommand.Name() && !viper.GetBool("password") {
				emailOptions := di.EmailClientOptions{EmailType: messengerOptions.MessengerType, MessengerOptions: &messengerOptions}
				emailMessengerClient, err = di.NewEmailClient(emailOptions)
				if err != nil {
					log.Ctx(ctx).Fatalf("error creating dashboard user client: %s", err.Error())
				}
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				log.Ctx(cmd.Context()).Fatalf("Error calling auth command: %s", err.Error())
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			// If the user was registered without set the password. We should
			// send the invitation email.
			if cmd.Name() == addUserSubcommand.Name() && !viper.GetBool("password") {
				ctx := cmd.Context()

				email, firstName := args[0], args[1]

				// We don't need to validate the content since it was already validated
				// in the stellar-auth
				role := viper.GetString("roles")
				tenantID := viper.GetString("tenant-id")
				if tenantID == "" {
					log.Ctx(ctx).Fatalf("tenant-id is required")
				}

				forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting forgot password link: %s", err.Error())
				}

				// 1. Get Tenant and save it in context.
				adminDSN, err := router.GetDSNForAdmin(globalOptions.DatabaseURL)
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting Admin DB DSN: %s", err.Error())
				}
				adminDBConnectionPool, err := db.OpenDBConnectionPool(adminDSN)
				if err != nil {
					log.Ctx(ctx).Fatalf("error opening Admin DB connection pool: %s", err.Error())
				}
				defer adminDBConnectionPool.Close()
				tm := tenant.NewManager(tenant.WithDatabase(adminDBConnectionPool))
				t, err := tm.GetTenantByID(ctx, tenantID)
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting tenant by id %s: %s", tenantID, err.Error())
				}
				ctx = tenant.SaveTenantInContext(ctx, t)

				// 2. Create user using multi-tenant connection pool
				tr := tenant.NewMultiTenantDataSourceRouter(tm)
				dbConnectionPool, err := db.NewConnectionPoolWithRouter(tr)
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting dbConnectionPool in execAddUser: %s", err.Error())
				}
				defer dbConnectionPool.Close()

				models, err := data.NewModels(dbConnectionPool)
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting models: %s", err.Error())
				}

				organization, err := models.Organizations.Get(ctx)
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting organization data: %s", err.Error())
				}

				invitationData := htmltemplate.InvitationMessageTemplate{
					FirstName:          firstName,
					Role:               role,
					ForgotPasswordLink: forgotPasswordLink,
					OrganizationName:   organization.Name,
				}

				msgBody, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(invitationData)
				if err != nil {
					log.Ctx(ctx).Fatalf("error executing invitation message template: %s", err.Error())
				}

				err = emailMessengerClient.SendMessage(message.Message{
					ToEmail: email,
					Title:   "Welcome to Stellar Disbursement Platform",
					Message: msgBody,
				})
				if err != nil {
					log.Ctx(ctx).Fatalf("error sending invitation message: %s", err.Error())
				}
			}
		},
	}

	if err := authCmdConfigOpts.Init(authCmd); err != nil {
		log.Ctx(authCmd.Context()).Fatalf("error initializing authCmd config options: %s", err.Error())
	}

	authCmd.AddCommand(addUserSubcommand)

	return authCmd
}
