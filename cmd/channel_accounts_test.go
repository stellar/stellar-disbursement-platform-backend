package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/cmd/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

func Test_ChannelAccountsCommand_CreateCommand(t *testing.T) {
	dbt := dbtest.Open(t)

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
		"--distribution-account-encryption-passphrase", keypair.MustRandom().Seed(),
		"--database-url", dbt.DSN,
	})

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		utils.AssertFuncExitsWithFatal(t, func() {
			customErr := errors.New("unexpected error creating channel accounts")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts create crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("CreateChannelAccounts", context.Background(), mock.Anything, 2).
				Return(customErr)

			_ = rootCmmd.Execute()
		}, "unexpected error creating channel accounts")
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
		"--distribution-account-encryption-passphrase", keypair.MustRandom().Seed(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
		"--database-url", dbt.DSN,
	})

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		utils.AssertFuncExitsWithFatal(t, func() {
			customErr := errors.New("unexpected error verifying channel accounts")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts verify crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("VerifyChannelAccounts", context.Background(), mock.Anything, false).
				Return(customErr).
				Once()

			_ = rootCmmd.Execute()
		}, "unexpected error verifying channel accounts")
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
		"--distribution-account-encryption-passphrase", keypair.MustRandom().Seed(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
		"--database-url", dbt.DSN,
	})

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		utils.AssertFuncExitsWithFatal(t, func() {
			customErr := errors.New("unexpected error ensuring channel accounts count")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts create crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("EnsureChannelAccountsCount", context.Background(), mock.Anything, 2).
				Return(customErr)

			_ = rootCmmd.Execute()
		}, "unexpected error ensuring channel accounts count")
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
		"--distribution-account-encryption-passphrase", keypair.MustRandom().Seed(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
		"--channel-account-id", "acc-id",
		"--database-url", dbt.DSN,
	}

	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
		rootCmmd.SetArgs(args)
		utils.AssertFuncExitsWithFatal(t, func() {
			customErr := errors.New("unexpected error deleting channel account")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts delete crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("DeleteChannelAccount", context.Background(), mock.Anything, mock.Anything).
				Return(customErr).
				Once()

			_ = rootCmmd.Execute()
		}, "unexpected error deleting channel account")
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
		defer di.ClearInstancesTestHelper(t)

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
		utils.AssertFuncExitsWithFatal(t, func() {
			customErr := errors.New("unexpected error viewing channel accounts")

			crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
			caCommand.CrashTrackerClient = crashTrackerMock
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts view crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			caServiceMock.
				On("ViewChannelAccounts", context.Background(), mock.Anything).
				Return(customErr).
				Once()
			defer caServiceMock.AssertExpectations(t)

			_ = parentCmdMock.Execute()
		}, "unexpected error viewing channel accounts")
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
