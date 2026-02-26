package cmd

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	cmdUtils "github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve"
	txSub "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission"
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
	dbt := dbtest.Open(t)
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	dbConnectionPool.Close()
	dbt.Close()

	cmdUtils.ClearTestEnvironment(t)

	// Pre-populate the DI container so PersistentPreRun doesn't create real DB connections.
	di.SetInstance(di.TSSDBConnectionPoolInstanceName, dbConnectionPool)
	di.SetInstance(di.AdminDBConnectionPoolInstanceName, dbConnectionPool)

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
		"--database-url", dbt.DSN,
		"--distribution-public-key", "GAXCC3VMCWRFZAFWK7JXI6M7XQ3WPVUUEL2SEFODWJY6N2QIFFGXSL6M",
		"--distribution-seed", "SBQ3ZNC2SE3FV43HZ2KW3FCXQMMIQ33LZB745KTMCHDS6PNQOVXMV5NC",
		"--channel-account-encryption-passphrase", "SDA3C7OW5HU4MMEEYTPXX43F4OU2MJBGF5WMJALL7CTILTI2GOVK2YFA",
		"--distribution-account-encryption-passphrase", "SDA3C7OW5HU4MMEEYTPXX43F4OU2MJBGF5WMJALL7CTILTI2GOVK2YFA",
		"--horizon-url", "https://horizon-testnet.stellar.org",
		"--network-passphrase", "Test SDF Network ; September 2015",
	})

	t.Setenv("DATABASE_URL", dbt.DSN)

	// test
	err = rootCmd.Execute()
	require.NoError(t, err)

	// assert
	mTSS.AssertExpectations(t)
}
