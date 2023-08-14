package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_persistentPostRun(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

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

	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	rootCmd.SetArgs([]string{"auth", "add-user", "email@email.com", "First", "Last", "--roles", "developer"})

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

	expectContains := `-------------------------------------------------------------------------------
Recipient: email@email.com
Subject: Welcome to Stellar Disbursement Platform
Content: <!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Welcome to Stellar Disbursement Platform</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
        }
        a {
            color: #3498db;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
        .button {
            display: inline-block;
            padding: 10px 20px;
            background-color: #3498db;
            color: #ffffff;
            text-decoration: none;
            border-radius: 5px;
            font-weight: bold;
        }
        .button:hover {
            background-color: #3cb0fd;
        }
    </style>
</head>
<body>
    <p>Hello, First!</p>
    <p>You have been added to your organization's Stellar Disbursement Platform as a developer. Please click the link below to set up your password and let your organization administrator know if you have any questions.</p>
    <p>
        <a href="http://localhost:3000/forgot-password" class="button">Set up my password</a>
    </p>
    <p>Best regards,</p>
    <p>The MyCustomAid Team</p>
</body>
</html>

-------------------------------------------------------------------------------
`

	w.Close()
	os.Stdout = stdOut

	buf := new(strings.Builder)
	_, err = io.Copy(buf, r)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), expectContains)

	// Set another SDP UI base URL
	rootCmd.SetArgs([]string{"auth", "add-user", "email@email.com", "First", "Last", "--roles", "developer", "--sdp-ui-base-url", "https://sdp-ui.org"})

	stdOut = os.Stdout

	r, w, err = os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	err = rootCmd.Execute()
	require.NoError(t, err)

	expectContains = `-------------------------------------------------------------------------------
Recipient: email@email.com
Subject: Welcome to Stellar Disbursement Platform
Content: <!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Welcome to Stellar Disbursement Platform</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
        }
        a {
            color: #3498db;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
        .button {
            display: inline-block;
            padding: 10px 20px;
            background-color: #3498db;
            color: #ffffff;
            text-decoration: none;
            border-radius: 5px;
            font-weight: bold;
        }
        .button:hover {
            background-color: #3cb0fd;
        }
    </style>
</head>
<body>
    <p>Hello, First!</p>
    <p>You have been added to your organization's Stellar Disbursement Platform as a developer. Please click the link below to set up your password and let your organization administrator know if you have any questions.</p>
    <p>
        <a href="https://sdp-ui.org/forgot-password" class="button">Set up my password</a>
    </p>
    <p>Best regards,</p>
    <p>The MyCustomAid Team</p>
</body>
</html>

-------------------------------------------------------------------------------
`

	w.Close()
	os.Stdout = stdOut

	buf.Reset()
	_, err = io.Copy(buf, r)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), expectContains)
}
