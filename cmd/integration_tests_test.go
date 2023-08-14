package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/integrationtests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockIntegrationTests struct {
	mock.Mock
}

// Making sure that mockServer implements ServerServiceInterface
var _ integrationtests.IntegrationTestsInterface = (*mockIntegrationTests)(nil)

func (m *mockIntegrationTests) StartIntegrationTests(ctx context.Context, opts integrationtests.IntegrationTestsOpts) error {
	return m.Called(ctx, opts).Error(0)
}

func (m *mockIntegrationTests) CreateTestData(ctx context.Context, opts integrationtests.IntegrationTestsOpts) error {
	return m.Called(ctx, opts).Error(0)
}

func Test_IntegrationTestsCommand_Command(t *testing.T) {
	dbt := dbtest.Open(t)

	command := &IntegrationTestsCommand{}

	root := rootCmd()
	cmd := command.Command()
	root.AddCommand(cmd)

	t.Setenv("DISBURSED_ASSET_CODE", "USDC")
	t.Setenv("DISBURSED_ASSET_ISSUER", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	t.Setenv("WALLET_NAME", "walletTest")
	t.Setenv("WALLET_HOMEPAGE", "https://www.test_wallet.com")
	t.Setenv("WALLET_DEEPLINK", "test_wallet://")

	root.SetArgs([]string{
		"integration-tests",
		"create-data",
		"--database-url",
		dbt.DSN,
	})
	err := cmd.Execute()
	require.NoError(t, err)
}

func Test_IntegrationTestsCommand_StartIntegrationTestsCommand(t *testing.T) {
	serviceMock := &mockIntegrationTests{}
	command := &IntegrationTestsCommand{Service: serviceMock}

	parentCmdMock := &cobra.Command{
		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	}

	integrationTestsOpts := &integrationtests.IntegrationTestsOpts{
		DatabaseDSN:                "randomDatabaseDSN",
		UserEmail:                  "mockemail@test.com",
		UserPassword:               "mockPassword123!",
		DisbursedAssetCode:         "USDC",
		DisbursetAssetIssuer:       "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
		WalletName:                 "walletTest",
		DisbursementCSVFilePath:    "mockPath",
		DisbursementCSVFileName:    "file.csv",
		ReceiverAccountPublicKey:   "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA",
		ReceiverAccountPrivateKey:  "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5",
		ReceiverAccountStellarMemo: "memo",
		Sep10SigningPublicKey:      "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S",
		RecaptchaSiteKey:           "reCAPTCHASiteKey",
		AnchorPlatformBaseSepURL:   "localhost:8080",
		ServerApiBaseURL:           "localhost:8000",
	}

	cmd := command.StartIntegrationTestsCommand(integrationTestsOpts)
	parentCmdMock.AddCommand(cmd)

	t.Setenv("DATABASE_URL", "randomDatabaseDSN")
	t.Setenv("USER_EMAIL", "mockemail@test.com")
	t.Setenv("USER_PASSWORD", "mockPassword123!")
	t.Setenv("DISBURSED_ASSET_CODE", "USDC")
	t.Setenv("DISBURSED_ASSET_ISSUER", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	t.Setenv("WALLET_NAME", "walletTest")
	t.Setenv("DISBURSEMENT_CSV_FILE_PATH", "mockPath")
	t.Setenv("DISBURSEMENT_CSV_FILE_NAME", "file.csv")
	t.Setenv("RECEIVER_ACCOUNT_PUBLIC_KEY", "GBC2HVWFIFN7WJHFORVBCDKJORG6LWTW3O2QBHOURL3KHZPM4KMWTUSA")
	t.Setenv("RECEIVER_ACCOUNT_PRIVATE_KEY", "SBUSPEKAZKLZSWHRSJ2HWDZUK6I3IVDUWA7JJZSGBLZ2WZIUJI7FPNB5")
	t.Setenv("RECEIVER_ACCOUNT_STELLAR_MEMO", "memo")
	t.Setenv("SEP10_SIGNING_PUBLIC_KEY", "GAX46JJZ3NPUM2EUBTTGFM6ITDF7IGAFNBSVWDONPYZJREHFPP2I5U7S")
	t.Setenv("RECAPTCHA_SITE_KEY", "reCAPTCHASiteKey")
	t.Setenv("ANCHOR_PLATFORM_BASE_SEP_URL", "localhost:8080")
	t.Setenv("SERVER_API_BASE_URL", "localhost:8000")

	parentCmdMock.SetArgs([]string{
		"start",
	})

	t.Run("exit with status 1 when IntegrationTestsService fails", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			serviceMock.
				On("StartIntegrationServe", context.Background(), *integrationTestsOpts).
				Return(errors.New("unexpected error"))

			err := parentCmdMock.Execute()
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

	t.Run("executes the start integration tests command successfully", func(t *testing.T) {
		serviceMock.
			On("StartIntegrationTests", context.Background(), *integrationTestsOpts).
			Return(nil)

		err := parentCmdMock.Execute()
		require.NoError(t, err)
	})

	serviceMock.AssertExpectations(t)
}

func Test_IntegrationTestsCommand_CreateIntegrationTestsDataCommand(t *testing.T) {
	serviceMock := &mockIntegrationTests{}
	command := &IntegrationTestsCommand{Service: serviceMock}

	parentCmdMock := &cobra.Command{
		PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	}

	integrationTestsOpts := &integrationtests.IntegrationTestsOpts{
		DatabaseDSN:          "randomDatabaseDSN",
		DisbursedAssetCode:   "USDC",
		DisbursetAssetIssuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV",
		WalletName:           "walletTest",
		WalletHomepage:       "https://www.test_wallet.com",
		WalletDeepLink:       "test_wallet://",
	}

	cmd := command.CreateIntegrationTestsDataCommand(integrationTestsOpts)
	parentCmdMock.AddCommand(cmd)

	t.Setenv("DATABASE_URL", "randomDatabaseDSN")
	t.Setenv("DISBURSED_ASSET_CODE", "USDC")
	t.Setenv("DISBURSED_ASSET_ISSUER", "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVV")
	t.Setenv("WALLET_NAME", "walletTest")
	t.Setenv("WALLET_HOMEPAGE", "https://www.test_wallet.com")
	t.Setenv("WALLET_DEEPLINK", "test_wallet://")

	parentCmdMock.SetArgs([]string{
		"create-data",
	})

	t.Run("exit with status 1 when IntegrationTestsService fails", func(t *testing.T) {
		if os.Getenv("TEST_FATAL") == "1" {
			serviceMock.
				On("CreateTestData", context.Background(), *integrationTestsOpts).
				Return(errors.New("unexpected error"))

			err := parentCmdMock.Execute()
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

	t.Run("executes the create integration tests data command successfully", func(t *testing.T) {
		serviceMock.
			On("CreateTestData", context.Background(), *integrationTestsOpts).
			Return(nil)

		err := parentCmdMock.Execute()
		require.NoError(t, err)
	})

	serviceMock.AssertExpectations(t)
}
