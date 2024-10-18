package cmd

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_persistentPostRun(t *testing.T) {
	tenantName := "tenant"
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	ctx := context.Background()

	adminDBConnectionPool := prepareAdminDBConnectionPool(t, ctx, dbt.DSN)
	defer adminDBConnectionPool.Close()

	tenant.PrepareDBForTenant(t, dbt, tenantName)
	tnt := tenant.CreateTenantFixture(t, ctx, adminDBConnectionPool, tenantName, "pub-key")

	t.Setenv("DATABASE_URL", dbt.DSN)
	t.Setenv("EMAIL_SENDER_TYPE", "DRY_RUN")

	addUserCmdMock := &cobra.Command{
		Use:  "add-user <email> <first name> <last name> [--password] [--owner]",
		Args: cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			assert.Equal(t, []string{"email@email.com", "First", "Last"}, args)
		},
	}

	addUserCmdMock.PersistentFlags().String("roles", "", "")
	err := viper.BindPFlag("roles", addUserCmdMock.PersistentFlags().Lookup("roles"))
	require.NoError(t, err)

	addUserCmdMock.PersistentFlags().String("tenant-id", "", "")
	err = viper.BindPFlag("tenant-id", addUserCmdMock.PersistentFlags().Lookup("tenant-id"))
	require.NoError(t, err)

	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	rootCmd.SetArgs([]string{"auth", "add-user", "email@email.com", "First", "Last", "--roles", "developer", "--tenant-id", tnt.ID})

	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "auth" {
			for _, authCmd := range cmd.Commands() {
				if authCmd.Name() == "add-user" {
					cmd.RemoveCommand(authCmd)
					cmd.AddCommand(addUserCmdMock)
					break
				}
			}
			break
		}
	}

	stdOut := os.Stdout

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	err = rootCmd.Execute()
	require.NoError(t, err)

	expectContainsSlice := []string{
		"<title>Welcome to Stellar Disbursement Platform</title>",
		"<p>You have been added to your organization's Stellar Disbursement Platform as a developer. Please click the button below to set up your password. If you have any questions, feel free to contact your organization administrator.</p>",
		`<a href="http://localhost:3000/forgot-password" class="button">Set up my password</a>`,
		"<p>Best regards,</p>",
		"<p>The MyCustomAid Team</p>",
	}

	w.Close()
	os.Stdout = stdOut

	buf := new(strings.Builder)
	_, err = io.Copy(buf, r)
	require.NoError(t, err)

	for _, expectContains := range expectContainsSlice {
		assert.Contains(t, buf.String(), expectContains)
	}

	// Set another SDP UI base URL
	rootCmd.SetArgs([]string{"auth", "add-user", "email@email.com", "First", "Last", "--roles", "developer", "--sdp-ui-base-url", "https://sdp-ui.org", "--tenant-id", tnt.ID})

	stdOut = os.Stdout

	r, w, err = os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	err = rootCmd.Execute()
	require.NoError(t, err)

	expectContainsSlice = []string{
		"<title>Welcome to Stellar Disbursement Platform</title>",
		"<p>You have been added to your organization's Stellar Disbursement Platform as a developer. Please click the button below to set up your password. If you have any questions, feel free to contact your organization administrator.</p>",
		`<a href="https://sdp-ui.org/forgot-password" class="button">Set up my password</a>`,
		"<p>Best regards,</p>",
		"<p>The MyCustomAid Team</p>",
	}

	w.Close()
	os.Stdout = stdOut

	buf.Reset()
	_, err = io.Copy(buf, r)
	require.NoError(t, err)

	for _, expectContains := range expectContainsSlice {
		assert.Contains(t, buf.String(), expectContains)
	}
}
