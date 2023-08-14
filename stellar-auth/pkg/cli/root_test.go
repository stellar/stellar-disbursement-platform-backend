package cli

import (
	"strings"
	"testing"

	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_rootCmd(t *testing.T) {
	testCases := []struct {
		name      string
		args      []string
		envVars   map[string]string
		expect    string
		notExpect string
	}{
		{
			name:   "test help command",
			args:   []string{"--help"},
			expect: "Stellar Auth handles JWT management.\n\nUsage:\n  stellarauth [flags]\n\nFlags:\n      --database-url string   Postgres DB URL (DATABASE_URL) (default \"postgres://postgres:postgres@localhost:5432/stellar-auth?sslmode=disable\")\n  -h, --help                  help for stellarauth\n      --log-level string      The log level used in this project. Options: \"TRACE\", \"DEBUG\", \"INFO\", \"WARN\", \"ERROR\", \"FATAL\", or \"PANIC\". (LOG_LEVEL) (default \"TRACE\")\n",
		},
		{
			name:   "test short help command",
			args:   []string{"-h"},
			expect: "Stellar Auth handles JWT management.\n\nUsage:\n  stellarauth [flags]\n\nFlags:\n      --database-url string   Postgres DB URL (DATABASE_URL) (default \"postgres://postgres:postgres@localhost:5432/stellar-auth?sslmode=disable\")\n  -h, --help                  help for stellarauth\n      --log-level string      The log level used in this project. Options: \"TRACE\", \"DEBUG\", \"INFO\", \"WARN\", \"ERROR\", \"FATAL\", or \"PANIC\". (LOG_LEVEL) (default \"TRACE\")\n",
		},
		{
			name:   "test set log-level",
			args:   []string{"--log-level", "INFO"},
			expect: "msg=\"GitCommit: \"",
		},
		{
			name:      "test set log-level with WARN level and doesn't logs INFO messages",
			args:      []string{"--log-level", "WARN"},
			expect:    "",
			notExpect: "msg=\"GitCommit: \"",
		},
		{
			name: "test set database-url",
			args: []string{"--log-level", "WARN", "--database-url", "postgres://localhost@5432/stellar-auth?sslmode=disable"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.envVars {
				t.Setenv(key, value)
			}

			rootCmd := rootCmd()
			rootCmd.SetArgs(tc.args)

			buf := new(strings.Builder)

			log.DefaultLogger.SetOutput(buf)
			rootCmd.SetOut(buf)

			err := rootCmd.Execute()
			require.NoError(t, err)

			output := buf.String()
			if tc.expect != "" {
				assert.Contains(t, output, tc.expect)
			}

			if tc.notExpect != "" {
				assert.NotContains(t, output, tc.notExpect)
			}
		})
	}
}

func Test_SetupCLI(t *testing.T) {
	cmd := SetupCLI("v0.0.1", "a1b2c3d4")

	buf := new(strings.Builder)
	cmd.SetOut(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "migrate     Apply Stellar Auth database migrations")
}
