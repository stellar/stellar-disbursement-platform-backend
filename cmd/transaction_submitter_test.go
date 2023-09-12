package cmd

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	txSub "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockSubmitter struct {
	mock.Mock
	wg sync.WaitGroup
}

func (t *mockSubmitter) StartSubmitter(ctx context.Context, opts txSub.SubmitterOptions) {
	t.Called(ctx, opts)
	t.wg.Wait()
}

func (t *mockSubmitter) StartMock(opts txSub.SubmitterOptions) {
	t.Called(opts)
}

func (t *mockSubmitter) StartMetricsServe(ctx context.Context, opts serve.MetricsServeOptions, httpServer serve.HTTPServerInterface, crashTrackerClient crashtracker.CrashTrackerClient) {
	t.Called(ctx, opts, httpServer, crashTrackerClient)
	t.wg.Done()
}

func Test_tss_help(t *testing.T) {
	// setup
	rootCmd := SetupCLI("x.y.z", "1234567890abcdef")
	tssCmdFound := false

	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "tss" {
			tssCmdFound = true
		}
	}
	require.True(t, tssCmdFound, "tss command not found")
	rootCmd.SetArgs([]string{"tss", "--help"})
	var out bytes.Buffer
	rootCmd.SetOut(&out)

	// test
	err := rootCmd.Execute()
	require.NoError(t, err)

	// assert
	assert.Contains(t, out.String(), "stellar-disbursement-platform tss [flags]", "should have printed help message for tss command")
}

func Test_tss(t *testing.T) {
	for _, env := range os.Environ() {
		key := env[:strings.Index(env, "=")]
		t.Setenv(key, "")
	}

	dryRunClient, err := crashtracker.NewDryRunClient()
	require.NoError(t, err)

	version := "x.y.z"
	gitCommitHash := "1234567890abcdef"

	mTSS := mockSubmitter{}
	rootCmd := SetupCLI(version, gitCommitHash)

	mTSS.On("StartMetricsServe", mock.Anything, mock.AnythingOfType("serve.MetricsServeOptions"), mock.AnythingOfType("*serve.HTTPServer"), dryRunClient).Once()
	mTSS.On("StartSubmitter", mock.Anything, mock.AnythingOfType("transactionsubmission.SubmitterOptions")).Once()
	mTSS.wg.Add(1)
	// setup
	var commandToRemove *cobra.Command
	commandToAdd := (&TxSubmitterCommand{}).Command(&mTSS)
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "tss" {
			commandToRemove = cmd
		}
	}

	require.NotNil(t, commandToRemove, "tss command not found")
	rootCmd.RemoveCommand(commandToRemove)
	rootCmd.AddCommand(commandToAdd)
	rootCmd.SetArgs([]string{
		"tss",
		"--environment", "test",
		"--database-url", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable",
		"--distribution-seed", "SBQ3ZNC2SE3FV43HZ2KW3FCXQMMIQ33LZB745KTMCHDS6PNQOVXMV5NC",
		"--horizon-url", "https://horizon-testnet.stellar.org",
		"--network-passphrase", "Test SDF Network ; September 2015",
	})

	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable")

	// test
	err = rootCmd.Execute()
	require.NoError(t, err)

	// assert
	mTSS.AssertExpectations(t)
}
