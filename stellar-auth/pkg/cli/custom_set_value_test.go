package cli

import (
	"go/types"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/stellar/go/support/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SetConfigOptionLogLevel(t *testing.T) {
	co := config.ConfigOption{
		Name:           "log-level",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionLogLevel,
	}

	executeCmd := func(args []string, handleError func(err error)) {
		// mock a command line argument
		testCmd := cobra.Command{
			Run: func(cmd *cobra.Command, args []string) {
				co.Require()
				// forward error to the error handler callback:
				handleError(co.SetValue())
			},
		}
		err := co.Init(&testCmd)
		require.NoError(t, err)

		// execute command line
		testCmd.SetArgs(args)
		err = testCmd.Execute()
		require.NoError(t, err)
	}

	// invalid log level should return an error
	testCount := 0
	executeCmd([]string{"--log-level", "aaa"}, func(err error) {
		require.EqualError(t, err, `couldn't parse log level: not a valid logrus Level: "aaa"`)
		testCount++
	})
	require.Equal(t, 1, testCount)

	// misconfigured configKey should return an error
	executeCmd([]string{"--log-level", "info"}, func(err error) {
		require.EqualError(t, err, `configKey has an invalid type <nil>`)
		testCount++
	})
	require.Equal(t, 2, testCount)

	// valid log level should set the configKey
	var logrusLevel logrus.Level
	require.NotEqual(t, logrus.InfoLevel, logrusLevel)
	co.ConfigKey = &logrusLevel
	executeCmd([]string{"--log-level", "info"}, func(err error) {
		require.NoError(t, err)
		testCount++
	})
	require.Equal(t, 3, testCount)
	require.Equal(t, logrus.InfoLevel, logrusLevel)

	// If no value is passed, stick with the default ("TRACE")
	co.FlagDefault = "TRACE"
	require.NotEqual(t, logrus.TraceLevel, logrusLevel)
	executeCmd(nil, func(err error) {
		require.NoError(t, err)
		testCount++
	})
	require.Equal(t, 4, testCount)
	require.Equal(t, logrus.TraceLevel, logrusLevel)
}

func Test_setConfigOptionRoles(t *testing.T) {
	var rolesConfigKey []string

	co := config.ConfigOption{
		Name:           "roles",
		OptType:        types.String,
		CustomSetValue: setConfigOptionRoles,
		ConfigKey:      &rolesConfigKey,
	}

	executeCmd := func(args []string, handleError func(err error)) {
		// mock a command line argument
		testCmd := cobra.Command{
			Run: func(cmd *cobra.Command, args []string) {
				co.Require()
				// forward error to the error handler callback:
				handleError(co.SetValue())
			},
		}
		err := co.Init(&testCmd)
		require.NoError(t, err)

		// execute command line
		testCmd.SetArgs(args)
		err = testCmd.Execute()
		require.NoError(t, err)
	}

	t.Run("handles set the roles through the CLI flag", func(t *testing.T) {
		testCount := 0
		executeCmd([]string{"--roles", "role1, role2, role3"}, func(err error) {
			require.NoError(t, err)
			testCount++
		})

		assert.Equal(t, []string{"role1", "role2", "role3"}, rolesConfigKey)

		executeCmd([]string{"--roles", "role1,role2,role3"}, func(err error) {
			require.NoError(t, err)
			testCount++
		})

		assert.Equal(t, []string{"role1", "role2", "role3"}, rolesConfigKey)

		executeCmd([]string{"--roles", ""}, func(err error) {
			require.NoError(t, err)
			testCount++
		})

		assert.Equal(t, []string{}, rolesConfigKey)
		assert.Equal(t, 3, testCount)
	})

	t.Run("handles set the roles through Env Vars", func(t *testing.T) {
		testCount := 0

		t.Setenv("ROLES", "role1, role2, role3")

		executeCmd([]string{}, func(err error) {
			require.NoError(t, err)
			testCount++
		})

		assert.Equal(t, []string{"role1", "role2", "role3"}, rolesConfigKey)

		t.Setenv("ROLES", "role1,role2,role3")

		executeCmd([]string{}, func(err error) {
			require.NoError(t, err)
			testCount++
		})

		assert.Equal(t, []string{"role1", "role2", "role3"}, rolesConfigKey)

		t.Setenv("ROLES", "")

		executeCmd([]string{"--roles", ""}, func(err error) {
			require.NoError(t, err)
			testCount++
		})

		assert.Equal(t, []string{}, rolesConfigKey)
		assert.Equal(t, 3, testCount)
	})
}
