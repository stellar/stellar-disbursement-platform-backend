package cmd

import (
	"bytes"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stretchr/testify/assert"
)

func Test_globalOptions_populateCrashTrackerOptions(t *testing.T) {
	globalOptions := globalOptionsType{
		environment: "test",
		gitCommit:   "1234567890abcdef",
		sentryDSN:   "test-sentry-dsn",
	}

	t.Run("CrashTrackerType is not Sentry", func(t *testing.T) {
		crashTrackerOptions := crashtracker.CrashTrackerOptions{}
		globalOptions.populateCrashTrackerOptions(&crashTrackerOptions)

		wantCrashTrackerOptions := crashtracker.CrashTrackerOptions{
			Environment: "test",
			GitCommit:   "1234567890abcdef",
		}
		assert.Equal(t, wantCrashTrackerOptions, crashTrackerOptions)
	})

	t.Run("CrashTrackerType is Sentry", func(t *testing.T) {
		crashTrackerOptions := crashtracker.CrashTrackerOptions{
			CrashTrackerType: crashtracker.CrashTrackerTypeSentry,
		}
		globalOptions.populateCrashTrackerOptions(&crashTrackerOptions)

		wantCrashTrackerOptions := crashtracker.CrashTrackerOptions{
			Environment:      "test",
			GitCommit:        "1234567890abcdef",
			SentryDSN:        "test-sentry-dsn",
			CrashTrackerType: crashtracker.CrashTrackerTypeSentry,
		}
		assert.Equal(t, wantCrashTrackerOptions, crashTrackerOptions)
	})
}

func Test_noArgsAndHelpHaveSameResultAndDoDontPanic(t *testing.T) {
	cmdArgsTestCases := [][]string{
		{"--help"},
		{},
	}

	for i, cmdArgs := range cmdArgsTestCases {
		// setup
		rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
		rootCmd.SetArgs(cmdArgs)
		var out bytes.Buffer
		rootCmd.SetOut(&out)

		// test
		err := rootCmd.Execute()
		assert.NoErrorf(t, err, "test case %d returned an error", i)

		// assert printed text
		assert.Containsf(t, out.String(), "Use \"stellar-disbursement-platform [command] --help\" for more information about a command.", "test case %d did not print help message as expected", i)
	}
}
