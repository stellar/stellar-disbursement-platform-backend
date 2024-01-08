package cli

import (
	"context"
	"fmt"
	"go/types"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

type PasswordPromptInterface interface {
	Run() (string, error)
}

var _ PasswordPromptInterface = (*promptui.Prompt)(nil)

var (
	isOwner      = false
	passwordFlag = false
)

func AddUserCmd(databaseURLFlagName string, passwordPrompt PasswordPromptInterface, availableRoles []string) *cobra.Command {
	var rolesConfigKey []string
	addUserCmdConfigOpts := config.ConfigOptions{
		{
			Name:        "owner",
			Usage:       `Set the user as Owner (superuser). Defaults to "false".`,
			OptType:     types.Bool,
			ConfigKey:   &isOwner,
			FlagDefault: false,
			Required:    true,
		},
		{
			Name:        "password",
			Usage:       "Sets the user password, it should be at least 8 characters long, if omitted, the command will generate a random one.",
			OptType:     types.Bool,
			ConfigKey:   &passwordFlag,
			FlagDefault: false,
			Required:    false,
		},
	}

	availableRolesDescription := ""
	if len(availableRoles) > 0 {
		availableRolesDescription = fmt.Sprintf("Available roles: [%s]", strings.Join(availableRoles, ", "))
		addUserCmdConfigOpts = append(addUserCmdConfigOpts, &config.ConfigOption{
			Name:           "roles",
			Usage:          fmt.Sprintf("Set the user roles. It should be comma separated. Example: role1, role2. %s.", availableRolesDescription),
			OptType:        types.String,
			CustomSetValue: setConfigOptionRoles,
			ConfigKey:      &rolesConfigKey,
			Required:       true,
		})
	}

	addUser := &cobra.Command{
		Use:   "add-user <email> <first name> <last name> [--owner] [--roles] [--password]",
		Short: "Add user to the system",
		Long:  "Add a user to the system. Email should be unique and password must be at least 8 characters long.",
		Args:  cobra.ExactArgs(3),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			if cmd.Parent().PersistentPreRun != nil {
				cmd.Parent().PersistentPreRun(cmd.Parent(), args)
				// Sending this cmd to its parents' PersistentPreRun, so that it can prepare the dependencies for wrapping up this command, if needed.
				cmd.Parent().PersistentPreRun(cmd, args)
			}

			addUserCmdConfigOpts.Require()
			err := addUserCmdConfigOpts.SetValues()
			if err != nil {
				log.Ctx(ctx).Fatalf("add-user error setting values of config options: %s", err.Error())
			}

			err = validateRoles(availableRoles, rolesConfigKey)
			if err != nil {
				log.Ctx(ctx).Fatalf("add-user error validating roles: %s", err.Error())
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()

			dbUrl := globalOptions.databaseURL
			if dbUrl == "" {
				dbUrl = viper.GetString(databaseURLFlagName)
			}

			email, firstName, lastName := args[0], args[1], args[2]

			var password string
			// If password flag is used, we prompt for a password.
			// Otherwise a OTP password is generated by the Auth Manager.
			if passwordFlag {
				result, err := passwordPrompt.Run()
				if err != nil {
					log.Fatalf("add-user error prompting password: %s", err)
				}

				pwValidator, err := utils.NewPasswordValidator()
				if err != nil {
					log.Fatalf("cannot initialize password validator: %s", err)

				}
				err = pwValidator.ValidatePassword(result)
				if err != nil {
					log.Fatalf("password is not valid: %v", err)
				}
				password = result
			}

			err := execAddUser(ctx, dbUrl, email, firstName, lastName, password, isOwner, rolesConfigKey)
			if err != nil {
				log.Fatalf("add-user command error: %s", err)
			}
			log.Infof("user inserted: %s", args[0])
		},
	}
	err := addUserCmdConfigOpts.Init(addUser)
	if err != nil {
		log.Fatalf("error initializing addUserCmd config option: %s", err.Error())
	}

	return addUser
}

// NewDefaultPasswordPrompt returns the default password prompt used in add-user command.
func NewDefaultPasswordPrompt() *promptui.Prompt {
	prompt := promptui.Prompt{
		Label: "Password",
		Mask:  ' ',
	}

	return &prompt
}

// execAddUser creates a new user and inserts it into the database, the user will have
// it's password encrypted for security reasons.
func execAddUser(ctx context.Context, dbUrl string, email, firstName, lastName, password string, isOwner bool, roles []string) error {
	dbConnectionPool, err := db.OpenDBConnectionPool(dbUrl)
	if err != nil {
		return fmt.Errorf("error getting dbConnectionPool in execAddUser: %w", err)
	}
	defer dbConnectionPool.Close()

	authManager := auth.NewAuthManager(
		auth.WithDefaultAuthenticatorOption(dbConnectionPool, auth.NewDefaultPasswordEncrypter(), 0),
	)

	newUser := &auth.User{
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
		IsOwner:   isOwner,
		Roles:     roles,
	}

	u, err := authManager.CreateUser(ctx, newUser, password)
	if err != nil {
		return fmt.Errorf("error creating user: %w", err)
	}

	log.Ctx(ctx).Infof("[CLI - CreateUserAccount] - Created user with account ID %s through CLI with roles %v", u.ID, roles)
	return nil
}

func validateRoles(availableRoles []string, rolesConfigKey []string) error {
	availableRolesMap := make(map[string]struct{}, len(availableRoles))
	for _, role := range availableRoles {
		availableRolesMap[role] = struct{}{}
	}

	for _, role := range rolesConfigKey {
		if _, ok := availableRolesMap[role]; !ok {
			return fmt.Errorf("invalid role provided. Expected one of these values: %s", strings.Join(availableRoles, " | "))
		}
	}

	return nil
}
