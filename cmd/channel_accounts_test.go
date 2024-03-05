package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	cmdDB "github.com/stellar/stellar-disbursement-platform-backend/cmd/db"
	"github.com/stellar/stellar-disbursement-platform-backend/cmd/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
)

func Test_ChannelAccountsCommand_Command(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	// Run tss migrations:
	globalOptions.DatabaseURL = dbt.DSN
	globalOptions.NetworkPassphrase = network.TestNetworkPassphrase

	root := rootCmd()
	dbCommand := (&cmdDB.DatabaseCommand{}).Command(&globalOptions)
	root.AddCommand(dbCommand)
	root.SetArgs([]string{
		"db",
		"tss",
		"migrate",
		"up",
		"--database-url", dbt.DSN,
	})
	err := dbCommand.Execute()
	require.NoError(t, err)

	// Run channel accounts verify:
	caCommand := (&ChannelAccountsCommand{}).Command(&ChAccCmdService{})
	root.AddCommand(caCommand)
	distributionKP := keypair.MustRandom()
	root.SetArgs([]string{
		"channel-accounts",
		"verify",
		"--database-url", dbt.DSN,
		"--distribution-seed", distributionKP.Seed(),
		"--distribution-public-key", distributionKP.Address(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
	})
	err = caCommand.Execute()
	require.NoError(t, err)
}

func Test_ChannelAccountsCommand_CreateCommand(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	distributionKP := keypair.MustRandom()
	globalOptions.NetworkPassphrase = network.TestNetworkPassphrase

	rootCmmd := rootCmd()
	caServiceMock := mocks.NewMockChAccCmdServiceInterface(t)
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	caCommand := (&ChannelAccountsCommand{CrashTrackerClient: crashTrackerMock}).Command(caServiceMock)
	rootCmmd.AddCommand(caCommand)
	rootCmmd.SetArgs([]string{
		"channel-accounts",
		"create", "2",
		"--distribution-seed", distributionKP.Seed(),
		"--distribution-public-key", distributionKP.Address(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
	})

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			customErr := errors.New("unexpected error")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts create crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("CreateChannelAccounts", context.Background(), mock.Anything, 2).
				Return(customErr)

			err := rootCmmd.Execute()
			require.NoError(t, err)

			return
		}

		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		cmd.Env = append(os.Environ(), "TEST_FATAL=1")
		err := cmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("executes the create command successfully", func(t *testing.T) {
		caServiceMock.
			On("CreateChannelAccounts", context.Background(), mock.Anything, 2).
			Return(nil)

		err := rootCmmd.Execute()
		require.NoError(t, err)
	})

	caServiceMock.AssertExpectations(t)
}

func Test_ChannelAccountsCommand_VerifyCommand(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	distributionKP := keypair.MustRandom()
	globalOptions.NetworkPassphrase = network.TestNetworkPassphrase

	rootCmmd := rootCmd()
	caServiceMock := mocks.NewMockChAccCmdServiceInterface(t)
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	caCommand := (&ChannelAccountsCommand{CrashTrackerClient: crashTrackerMock}).Command(caServiceMock)
	rootCmmd.AddCommand(caCommand)
	rootCmmd.SetArgs([]string{
		"channel-accounts",
		"verify",
		"--distribution-seed", distributionKP.Seed(),
		"--distribution-public-key", distributionKP.Address(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
	})

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			customErr := errors.New("unexpected error")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts verify crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("VerifyChannelAccounts", context.Background(), mock.Anything, false).
				Return(customErr).
				Once()

			err := rootCmmd.Execute()
			require.NoError(t, err)

			return
		}

		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

		err := cmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("executes the verify command successfully", func(t *testing.T) {
		caServiceMock.
			On("VerifyChannelAccounts", context.Background(), mock.Anything, false).
			Return(nil).
			Once()

		err := rootCmmd.Execute()
		require.NoError(t, err)
	})

	caServiceMock.AssertExpectations(t)
}

func Test_ChannelAccountsCommand_EnsureCommand(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	distributionKP := keypair.MustRandom()
	globalOptions.NetworkPassphrase = network.TestNetworkPassphrase

	rootCmmd := rootCmd()
	caServiceMock := mocks.NewMockChAccCmdServiceInterface(t)
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	caCommand := (&ChannelAccountsCommand{CrashTrackerClient: crashTrackerMock}).Command(caServiceMock)
	rootCmmd.AddCommand(caCommand)
	rootCmmd.SetArgs([]string{
		"channel-accounts",
		"ensure", "2",
		"--distribution-seed", distributionKP.Seed(),
		"--distribution-public-key", distributionKP.Address(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
	})

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			customErr := errors.New("unexpected error")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts create crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("EnsureChannelAccountsCount", context.Background(), mock.Anything, 2).
				Return(customErr)

			err := rootCmmd.Execute()
			require.NoError(t, err)

			return
		}

		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		cmd.Env = append(os.Environ(), "TEST_FATAL=1")
		err := cmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("executes the create command successfully", func(t *testing.T) {
		caServiceMock.
			On("EnsureChannelAccountsCount", context.Background(), mock.Anything, 2).
			Return(nil)

		err := rootCmmd.Execute()
		require.NoError(t, err)
	})

	caServiceMock.AssertExpectations(t)
}

func Test_ChannelAccountsCommand_DeleteCommand(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	distributionKP := keypair.MustRandom()
	globalOptions.NetworkPassphrase = network.TestNetworkPassphrase

	rootCmmd := rootCmd()
	caServiceMock := mocks.NewMockChAccCmdServiceInterface(t)
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	caCommand := (&ChannelAccountsCommand{CrashTrackerClient: crashTrackerMock}).Command(caServiceMock)
	rootCmmd.AddCommand(caCommand)

	args := []string{
		"channel-accounts",
		"delete",
		"--distribution-seed", distributionKP.Seed(),
		"--distribution-public-key", distributionKP.Address(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
		"--channel-account-id", "acc-id",
	}

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		rootCmmd.SetArgs(args)
		if os.Getenv("TEST_FATAL") == "1" {
			customErr := errors.New("unexpected error")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts delete crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("DeleteChannelAccount", context.Background(), mock.Anything, mock.Anything).
				Return(customErr).
				Once()

			err := rootCmmd.Execute()
			require.NoError(t, err)

			return
		}

		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

		err := cmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("executes the delete command successfully", func(t *testing.T) {
		rootCmmd.SetArgs(args)
		caServiceMock.
			On("DeleteChannelAccount", context.Background(), mock.Anything, mock.Anything).
			Return(nil).
			Once()

		err := rootCmmd.Execute()
		require.NoError(t, err)
	})

	t.Run("delete command fails when both channel-account-id and delete-all-accounts are set", func(t *testing.T) {
		rootCmmd.SetArgs(append(args, "--delete-all-accounts"))

		err := rootCmmd.Execute()
		require.EqualError(
			t,
			err,
			"if any flags in the group [channel-account-id delete-all-accounts] are set none of the others can be; [channel-account-id delete-all-accounts] were all set",
		)
	})

	caServiceMock.AssertExpectations(t)
}

func Test_ChannelAccountsCommand_ViewCommand(t *testing.T) {
	parentCmdMock := &cobra.Command{PersistentPreRun: func(cmd *cobra.Command, args []string) {}}
	parentCmdMock.SetArgs([]string{"view"})

	caServiceMock := mocks.NewMockChAccCmdServiceInterface(t)
	caCommand := &ChannelAccountsCommand{}
	cmd := caCommand.ViewCommand(caServiceMock)
	parentCmdMock.AddCommand(cmd)

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			customErr := errors.New("unexpected error")

			crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
			caCommand.CrashTrackerClient = crashTrackerMock
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts view crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("ViewChannelAccounts", context.Background()).
				Return(customErr).
				Once()
			defer caServiceMock.AssertExpectations(t)

			err := parentCmdMock.Execute()
			require.NoError(t, err)

			return
		}

		// We're using a strategy to setup a innerCmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
		// Ref: https://go.dev/talks/2014/testing.slide#23
		innerCmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
		innerCmd.Env = append(os.Environ(), "TEST_FATAL=1")

		err := innerCmd.Run()
		if exitError, ok := err.(*exec.ExitError); ok {
			assert.False(t, exitError.Success())
			return
		}

		t.Fatalf("process ran with err %v, want exit status 1", err)
	})

	t.Run("executes the list command successfully", func(t *testing.T) {
		caCommand.CrashTrackerClient = nil
		caServiceMock.
			On("ViewChannelAccounts", context.Background(), mock.Anything).
			Return(nil).
			Once()

		err := parentCmdMock.Execute()
		require.NoError(t, err)
	})

	caServiceMock.AssertExpectations(t)
}
