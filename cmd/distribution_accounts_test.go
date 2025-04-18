package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	sigMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_DistributionAccountCommand_RotateCommand(t *testing.T) {
	ctx := context.Background()
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	adminDBConnectionPool := prepareAdminDBConnectionPool(t, ctx, dbt.DSN)
	defer adminDBConnectionPool.Close()

	oldAccount := schema.NewDefaultStellarTransactionAccount(keypair.MustRandom().Address())
	hostAccountKP := keypair.MustRandom()

	tenantName := "tenant1"
	tenant.PrepareDBForTenant(t, dbt, tenantName)
	testTenant := tenant.CreateTenantFixture(t, ctx, adminDBConnectionPool, tenantName, oldAccount.Address)
	ctx = tenant.SaveTenantInContext(ctx, testTenant)

	globalOptions.NetworkPassphrase = network.TestNetworkPassphrase

	rootCmmd := rootCmd()
	crashTrackerMock := &crashtracker.MockCrashTrackerClient{}
	distAccountResolverMock := sigMocks.NewMockDistributionAccountResolver(t)

	// Create the command
	distAccountCommand := &DistributionAccountCommand{
		CrashTrackerClient:    crashTrackerMock,
		DistAccResolver:       distAccountResolverMock,
		AdminDBConnectionPool: adminDBConnectionPool,
		TSSDBConnectionPool:   dbConnectionPool,
	}

	// Add to root command
	cmdService := &MockDistAccCmdServiceInterface{}
	rootCmmd.AddCommand(distAccountCommand.Command(cmdService))

	// Setup the test arguments
	rootCmmd.SetArgs([]string{
		"distribution-account",
		"rotate",
		"--tenant-id", testTenant.ID,
		"--distribution-public-key", hostAccountKP.Address(),
		"--database-url", dbt.DSN,
		"--max-base-fee", "100",
		"--network-passphrase", network.TestNetworkPassphrase,
		"--tenant-xlm-bootstrap-amount", "5",
		"--distribution-account-encryption-passphrase", hostAccountKP.Seed(),
		"--channel-account-encryption-passphrase", hostAccountKP.Seed(),
		"--distribution-seed", hostAccountKP.Seed(),
	})

	t.Run("🎉 successfully executes the rotate command", func(t *testing.T) {
		cmdService.
			On("RotateDistributionAccount", ctx, mock.AnythingOfType("DistributionAccountService")).
			Return(nil).
			Once()

		rootCmd().SetContext(ctx)
		err := rootCmmd.Execute()
		require.NoError(t, err)
	})

	t.Run("exit with status 1 when DistributionAccountService fails", func(t *testing.T) {
		utils.AssertFuncExitsWithFatal(t, func() {
			customErr := errors.New("unexpected error")
			crashTrackerMock.
				On("LogAndReportErrors", context.Background(), customErr, "Cmd distribution-accounts rotate crash").
				Once()
			defer crashTrackerMock.AssertExpectations(t)

			cmdService.
				On("RotateDistributionAccount", ctx, mock.AnythingOfType("DistributionAccountService")).
				Return(customErr).
				Once()

			rootCmd().SetContext(ctx)
			_ = rootCmmd.Execute()
		}, "Error rotating distribution account: unexpected error")
	})
}
