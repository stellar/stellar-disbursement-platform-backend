package cmd

import (
	"fmt"
	"go/types"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
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
			Usage:          "The SDP UI Base URL used to send the invitation link when a new user is created.",
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
	addUserSubcommand := cli.AddUserCmd(dbConfigOptionFlagName, cli.NewDefaultPasswordPrompt(), availableRoles)

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
				log.Fatalf("error setting values of config options: %s", err.Error())
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
				log.Fatalf("Error calling auth command: %s", err.Error())
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

				forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting forgot password link: %s", err.Error())
				}

				dbConnectionPool, err := db.OpenDBConnectionPool(globalOptions.databaseURL)
				if err != nil {
					log.Ctx(ctx).Fatalf("error getting database connection: %s", err.Error())
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
		log.Fatalf("error initializing authCmd config options: %s", err.Error())
	}

	authCmd.AddCommand(addUserSubcommand)

	return authCmd
}
