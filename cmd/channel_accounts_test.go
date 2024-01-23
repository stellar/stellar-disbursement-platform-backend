package cmd

import (
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/require"

	cmdDB "github.com/stellar/stellar-disbursement-platform-backend/cmd/db"
	"github.com/stellar/stellar-disbursement-platform-backend/cmd/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_ChannelAccountsCommand_Command(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	root := rootCmd()

	// Run tss migrations:
	globalOptions := utils.GlobalOptionsType{
		DatabaseURL:       dbt.DSN,
		NetworkPassphrase: network.TestNetworkPassphrase,
	}
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
	caCommand := (&ChannelAccountsCommand{}).Command()
	root.AddCommand(caCommand)
	root.SetArgs([]string{
		"channel-accounts",
		"verify",
		"--database-url", dbt.DSN,
		"--distribution-seed", keypair.MustRandom().Seed(),
		"--channel-account-encryption-passphrase", keypair.MustRandom().Seed(),
	})
	err = caCommand.Execute()
	require.NoError(t, err)
}

// func Test_ChannelAccountsCommand_CreateCommand(t *testing.T) {
// 	dbt := dbtest.Open(t)
// 	defer dbt.Close()
// 	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
// 	require.NoError(t, outerErr)
// 	defer dbConnectionPool.Close()

// 	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
// 	caCommand := &ChannelAccountsCommand{
// 		CrashTrackerClient:  crashTrackerMock,
// 		TSSDBConnectionPool: dbConnectionPool,
// 	}

// 	parentCmdMock := &cobra.Command{
// 		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
// 	}

// 	cmd := caCommand.CreateCommand()
// 	parentCmdMock.AddCommand(cmd)

// 	distributionSeed := keypair.MustRandom().Seed()

// 	parentCmdMock.SetArgs([]string{
// 		"create", "2",
// 		"--distribution-seed",
// 		distributionSeed,
// 	})

// t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
// 	if os.Getenv("TEST_FATAL") == "1" {
// 		customErr := errors.New("unexpected error")
// 		caServiceMock.
// 			On("CreateChannelAccountsOnChain", context.Background(), txSubSvc.ChannelAccountServiceOptions{
// 				NumChannelAccounts: 2,
// 				MaxBaseFee:         txnbuild.MinBaseFee,
// 				RootSeed:           distributionSeed,
// 			}).
// 			Return(customErr)
// 		crashTrackerMock.On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts create crash")

// 		err := parentCmdMock.Execute()
// 		require.NoError(t, err)

// 		return
// 	}

// 	// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
// 	// Ref: https://go.dev/talks/2014/testing.slide#23
// 	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
// 	cmd.Env = append(os.Environ(), "TEST_FATAL=1")
// 	err := cmd.Run()
// 	if exitError, ok := err.(*exec.ExitError); ok {
// 		assert.False(t, exitError.Success())
// 		return
// 	}

// 	t.Fatalf("process ran with err %v, want exit status 1", err)
// })

// t.Run("executes the create command successfully", func(t *testing.T) {
// 	caServiceMock.
// 		On("CreateChannelAccountsOnChain", context.Background(), txSubSvc.ChannelAccountServiceOptions{
// 			NumChannelAccounts: 2,
// 			MaxBaseFee:         100 * txnbuild.MinBaseFee,
// 			RootSeed:           distributionSeed,
// 		}).
// 		Return(nil)

// 	err := parentCmdMock.Execute()
// 	require.NoError(t, err)
// })

// caServiceMock.AssertExpectations(t)
// crashTrackerMock.AssertExpectations(t)
// }

// func Test_ChannelAccountsCommand_VerifyCommand(t *testing.T) {
// 	caServiceMock := &txSubSvc.ChannelAccountsServiceMock{}
// 	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
// 	caCommand := &ChannelAccountsCommand{Service: caServiceMock}

// 	parentCmdMock := &cobra.Command{
// 		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
// 	}

// 	cmd := caCommand.VerifyCommand(&txSubSvc.ChannelAccountServiceOptions{})
// 	parentCmdMock.AddCommand(cmd)

// 	parentCmdMock.SetArgs([]string{
// 		"verify",
// 	})

// 	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
// 		if os.Getenv("TEST_FATAL") == "1" {
// 			customErr := errors.New("unexpected error")
// 			caServiceMock.
// 				On("VerifyChannelAccounts", context.Background()).
// 				Return(customErr)
// 			crashTrackerMock.On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts verify crash")

// 			err := parentCmdMock.Execute()
// 			require.NoError(t, err)

// 			return
// 		}

// 		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
// 		// Ref: https://go.dev/talks/2014/testing.slide#23
// 		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
// 		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

// 		err := cmd.Run()
// 		if exitError, ok := err.(*exec.ExitError); ok {
// 			assert.False(t, exitError.Success())
// 			return
// 		}

// 		t.Fatalf("process ran with err %v, want exit status 1", err)
// 	})

// 	t.Run("executes the verify command successfully", func(t *testing.T) {
// 		caServiceMock.
// 			On("VerifyChannelAccounts", context.Background()).
// 			Return(nil)

// 		err := parentCmdMock.Execute()
// 		require.NoError(t, err)
// 	})

// 	caServiceMock.AssertExpectations(t)
// 	crashTrackerMock.AssertExpectations(t)
// }

// func Test_ChannelAccountsCommand_EnsureCommand(t *testing.T) {
// 	caServiceMock := &txSubSvc.ChannelAccountsServiceMock{}
// 	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
// 	caCommand := &ChannelAccountsCommand{Service: caServiceMock}

// 	parentCmdMock := &cobra.Command{
// 		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
// 	}

// 	cmd := caCommand.EnsureCommand(&txSubSvc.ChannelAccountServiceOptions{})
// 	parentCmdMock.AddCommand(cmd)

// 	distributionSeed := keypair.MustRandom().Seed()

// 	parentCmdMock.SetArgs([]string{
// 		"ensure",
// 		"--distribution-seed",
// 		distributionSeed,
// 		"--num-channel-accounts-ensure",
// 		"2",
// 	})

// 	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
// 		if os.Getenv("TEST_FATAL") == "1" {
// 			customErr := errors.New("unexpected error")
// 			caServiceMock.
// 				On("EnsureChannelAccountsCount", context.Background(), txSubSvc.ChannelAccountServiceOptions{
// 					MaxBaseFee:         txnbuild.MinBaseFee,
// 					NumChannelAccounts: 2,
// 					RootSeed:           distributionSeed,
// 				}).
// 				Return(customErr)
// 			crashTrackerMock.On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts ensure crash")

// 			err := parentCmdMock.Execute()
// 			require.NoError(t, err)

// 			return
// 		}

// 		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
// 		// Ref: https://go.dev/talks/2014/testing.slide#23
// 		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
// 		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

// 		err := cmd.Run()
// 		if exitError, ok := err.(*exec.ExitError); ok {
// 			assert.False(t, exitError.Success())
// 			return
// 		}

// 		t.Fatalf("process ran with err %v, want exit status 1", err)
// 	})

// 	t.Run("executs the ensure command successfully", func(t *testing.T) {
// 		caServiceMock.
// 			On("EnsureChannelAccountsCount", context.Background(), txSubSvc.ChannelAccountServiceOptions{
// 				MaxBaseFee:         100 * txnbuild.MinBaseFee,
// 				NumChannelAccounts: 2,
// 				RootSeed:           distributionSeed,
// 			}).
// 			Return(nil)

// 		err := parentCmdMock.Execute()
// 		require.NoError(t, err)
// 	})

// 	caServiceMock.AssertExpectations(t)
// 	crashTrackerMock.AssertExpectations(t)
// }

// func Test_ChannelAccountsCommand_DeleteCommand(t *testing.T) {
// 	caServiceMock := &txSubSvc.ChannelAccountsServiceMock{}
// 	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
// 	caCommand := &ChannelAccountsCommand{Service: caServiceMock}

// 	parentCmdMock := &cobra.Command{
// 		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
// 	}

// 	cmd := caCommand.DeleteCommand(&txSubSvc.ChannelAccountServiceOptions{})
// 	parentCmdMock.AddCommand(cmd)

// 	distributionSeed := keypair.MustRandom().Seed()

// 	args := []string{
// 		"delete",
// 		"--distribution-seed",
// 		distributionSeed,
// 		"--channel-account-id",
// 		"acc-id",
// 	}

// 	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
// 		parentCmdMock.SetArgs(args)
// 		customErr := errors.New("unexpected error")
// 		if os.Getenv("TEST_FATAL") == "1" {
// 			caServiceMock.
// 				On("DeleteChannelAccount", context.Background(), txSubSvc.ChannelAccountServiceOptions{
// 					MaxBaseFee:       txnbuild.MinBaseFee,
// 					ChannelAccountID: "acc-id",
// 					RootSeed:         distributionSeed,
// 				}).
// 				Return(customErr)
// 			crashTrackerMock.On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts delete crash")

// 			err := parentCmdMock.Execute()
// 			require.NoError(t, err)

// 			return
// 		}

// 		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
// 		// Ref: https://go.dev/talks/2014/testing.slide#23
// 		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
// 		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

// 		err := cmd.Run()
// 		if exitError, ok := err.(*exec.ExitError); ok {
// 			assert.False(t, exitError.Success())
// 			return
// 		}

// 		t.Fatalf("process ran with err %v, want exit status 1", err)
// 	})

// 	t.Run("executes the delete command successfully", func(t *testing.T) {
// 		parentCmdMock.SetArgs(args)
// 		caServiceMock.
// 			On("DeleteChannelAccount", context.Background(), txSubSvc.ChannelAccountServiceOptions{
// 				MaxBaseFee:       100 * txnbuild.MinBaseFee,
// 				ChannelAccountID: "acc-id",
// 				RootSeed:         distributionSeed,
// 			}).
// 			Return(nil)

// 		err := parentCmdMock.Execute()
// 		require.NoError(t, err)
// 	})

// 	t.Run("delete command fails when both channel-account-id and delete-all-accounts are set", func(t *testing.T) {
// 		parentCmdMock.SetArgs(append(args, "--delete-all-accounts"))

// 		err := parentCmdMock.Execute()
// 		require.EqualError(
// 			t,
// 			err,
// 			"if any flags in the group [channel-account-id delete-all-accounts] are set none of the others can be; [channel-account-id delete-all-accounts] were all set",
// 		)
// 	})

// 	caServiceMock.AssertExpectations(t)
// 	crashTrackerMock.AssertExpectations(t)
// }

// func Test_ChannelAccountsCommand_ViewCommand(t *testing.T) {
// 	caServiceMock := &txSubSvc.ChannelAccountsServiceMock{}
// 	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
// 	caCommand := &ChannelAccountsCommand{Service: caServiceMock}

// 	parentCmdMock := &cobra.Command{
// 		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
// 	}

// 	cmd := caCommand.ViewCommand()
// 	parentCmdMock.AddCommand(cmd)

// 	parentCmdMock.SetArgs([]string{
// 		"view",
// 	})

// 	t.Run("exit with status 1 when ChannelAccountsService fails", func(t *testing.T) {
// 		if os.Getenv("TEST_FATAL") == "1" {
// 			customErr := errors.New("unexpected error")
// 			caServiceMock.
// 				On("ViewChannelAccounts", context.Background()).
// 				Return(errors.New("unexpected error"))
// 			crashTrackerMock.On("LogAndReportErrors", context.Background(), customErr, "Cmd channel-accounts view crash")

// 			err := parentCmdMock.Execute()
// 			require.NoError(t, err)

// 			return
// 		}

// 		// We're using a strategy to setup a cmd inside the test that calls the test itself and verifies if it exited with exit status '1'.
// 		// Ref: https://go.dev/talks/2014/testing.slide#23
// 		cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", t.Name()))
// 		cmd.Env = append(os.Environ(), "TEST_FATAL=1")

// 		err := cmd.Run()
// 		if exitError, ok := err.(*exec.ExitError); ok {
// 			assert.False(t, exitError.Success())
// 			return
// 		}

// 		t.Fatalf("process ran with err %v, want exit status 1", err)
// 	})

// 	t.Run("executes the view command successfully", func(t *testing.T) {
// 		caServiceMock.
// 			On("ViewChannelAccounts", context.Background()).
// 			Return(nil)

// 		err := parentCmdMock.Execute()
// 		require.NoError(t, err)
// 	})

// 	caServiceMock.AssertExpectations(t)
// 	crashTrackerMock.AssertExpectations(t)
// }
