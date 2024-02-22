package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	preconditionsMock "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/preconditions/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_validateTenantNameArg(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{
			name: "orgname",
			err:  nil,
		},
		{
			name: "orgname-ukraine",
			err:  nil,
		},
		{
			name: "ORGNAME",
			err:  errors.New(`invalid tenant name "ORGNAME". It should only contains lower case letters and dash (-)`),
		},
		{
			name: "orgname org",
			err:  errors.New(`invalid tenant name "orgname org". It should only contains lower case letters and dash (-)`),
		},
		{
			name: "orgname126",
			err:  errors.New(`invalid tenant name "orgname126". It should only contains lower case letters and dash (-)`),
		},
		{
			name: "@rgn#ame$",
			err:  errors.New(`invalid tenant name "@rgn#ame$". It should only contains lower case letters and dash (-)`),
		},
		{
			name: "orgname_ukraine",
			err:  errors.New(`invalid tenant name "orgname_ukraine". It should only contains lower case letters and dash (-)`),
		},
	}

	for _, tc := range testCases {
		err := validateTenantNameArg(&cobra.Command{}, []string{tc.name})
		if tc.err != nil {
			assert.Equal(t, tc.err, err)
		} else {
			assert.Nil(t, err)
		}
	}
}

func Test_executeAddTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	messengerClientMock := &message.MessengerClientMock{}
	messengerClientMock.
		On("SendMessage", mock.AnythingOfType("message.Message")).
		Return(nil)

	ctx := context.Background()

	tenantName := "myorg"
	userFirstName := "First"
	userLastName := "Last"
	userEmail := "email@email.com"
	organizationName := "My Org"
	uiBaseURL := "http://localhost:3000"
	networkType := "testnet"
	horizonURL := "https://horizon-testnet.stellar.org"
	encryptionPassphrase := keypair.MustRandom().Seed()
	distributionAccPrivKey := keypair.MustRandom().Seed()

	sigOpts := signing.SignatureServiceOptions{
		DistributionSignerType: signing.DistributionAccountEnvSignatureClientType,
		EncryptionPassphrase:   encryptionPassphrase,
		DistributionPrivateKey: distributionAccPrivKey,
		NetworkPassphrase:      network.TestNetworkPassphrase,
		DBConnectionPool:       dbConnectionPool,
		LedgerNumberTracker:    &preconditionsMock.MockLedgerNumberTracker{},
	}
	tenantsOpts := AddTenantsCommandOptions{
		SDPUIBaseURL: &uiBaseURL,
		NetworkType:  networkType,
		HorizonURL:   horizonURL,
	}

	t.Run("adds a new tenant successfully", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := executeAddTenant(ctx, dbt.DSN, tenantName, userFirstName, userLastName, userEmail, organizationName, messengerClientMock, tenantsOpts, sigOpts)
		assert.Nil(t, err)

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, tenantName)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 20)
		assert.Equal(t, "tenant myorg added successfully", entries[18].Message)
		assert.Contains(t, fmt.Sprintf("tenant ID: %s", tenantID), entries[19].Message)
	})

	t.Run("duplicated tenant name", func(t *testing.T) {
		tenant.DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		err := executeAddTenant(ctx, dbt.DSN, tenantName, userFirstName, userLastName, userEmail, organizationName, messengerClientMock, tenantsOpts, sigOpts)
		assert.Nil(t, err)

		err = executeAddTenant(ctx, dbt.DSN, tenantName, userFirstName, userLastName, userEmail, organizationName, messengerClientMock, tenantsOpts, sigOpts)
		assert.ErrorIs(t, err, tenant.ErrDuplicatedTenantName)

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, tenantName)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 18)
		assert.Equal(t, "tenant myorg added successfully", entries[15].Message)
		assert.Contains(t, fmt.Sprintf("tenant ID: %s", tenantID), entries[16].Message)
	})

	messengerClientMock.AssertExpectations(t)
}

func Test_AddTenantsCmd(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	t.Setenv("DISTRIBUTION_SEED", keypair.MustRandom().Seed())
	t.Setenv("CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE", keypair.MustRandom().Seed())
	t.Setenv("HORIZON_URL", "https://horizon-testnet.stellar.org/")

	t.Run("shows usage", func(t *testing.T) {
		out := new(strings.Builder)
		mockCmd := cobra.Command{}
		mockCmd.AddCommand(AddTenantsCmd())
		mockCmd.SetOut(out)
		mockCmd.SetErr(out)
		mockCmd.SetArgs([]string{"add-tenants"})
		err := mockCmd.ExecuteContext(ctx)
		assert.EqualError(t, err, "accepts 5 arg(s), received 0")

		expectUsageMessage := `Error: accepts 5 arg(s), received 0
Usage:
   add-tenants [flags]

Examples:
add-tenants [tenant name] [user first name] [user last name] [user email] [organization name]

Flags:
      --aws-access-key-id string                       The AWS access key ID (AWS_ACCESS_KEY_ID)
      --aws-region string                              The AWS region (AWS_REGION)
      --aws-secret-access-key string                   The AWS secret access key (AWS_SECRET_ACCESS_KEY)
      --channel-account-encryption-passphrase string   A Stellar-compliant ed25519 private key used to encrypt/decrypt the channel accounts' private keys. When not set, it will default to the value of the 'distribution-seed' option. (CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE)
      --distribution-seed string                       The private key of the Stellar distribution account that sends the disbursements. (DISTRIBUTION_SEED)
      --distribution-signer-type string                The type of the signature client used for distribution accounts. Options: [DISTRIBUTION_ACCOUNT_ENV] (DISTRIBUTION_SIGNER_TYPE) (default "DISTRIBUTION_ACCOUNT_ENV")
      --email-sender-type string                       The messenger type used to send invitations to new dashboard users. Options: [DRY_RUN AWS_EMAIL] (EMAIL_SENDER_TYPE)
  -h, --help                                           help for add-tenants
      --network-passphrase string                      The Stellar Network passphrase (NETWORK_PASSPHRASE) (default "Test SDF Network ; September 2015")
      --network-type string                            The Stellar Network type (NETWORK_TYPE) (default "testnet")
      --sdp-ui-base-url string                         The Tenant SDP UI/dashboard Base URL. (SDP_UI_BASE_URL) (default "http://localhost:3000")

`
		assert.Equal(t, expectUsageMessage, out.String())

		out.Reset()
		mockCmd.SetArgs([]string{"add-tenants", "--help"})
		err = mockCmd.ExecuteContext(ctx)
		require.NoError(t, err)

		expectUsageMessage = `Add a new tenant. The tenant name should only contain lower case characters and dash (-)

Usage:
   add-tenants [flags]

Examples:
add-tenants [tenant name] [user first name] [user last name] [user email] [organization name]

Flags:
      --aws-access-key-id string                       The AWS access key ID (AWS_ACCESS_KEY_ID)
      --aws-region string                              The AWS region (AWS_REGION)
      --aws-secret-access-key string                   The AWS secret access key (AWS_SECRET_ACCESS_KEY)
      --channel-account-encryption-passphrase string   A Stellar-compliant ed25519 private key used to encrypt/decrypt the channel accounts' private keys. When not set, it will default to the value of the 'distribution-seed' option. (CHANNEL_ACCOUNT_ENCRYPTION_PASSPHRASE)
      --distribution-seed string                       The private key of the Stellar distribution account that sends the disbursements. (DISTRIBUTION_SEED)
      --distribution-signer-type string                The type of the signature client used for distribution accounts. Options: [DISTRIBUTION_ACCOUNT_ENV] (DISTRIBUTION_SIGNER_TYPE) (default "DISTRIBUTION_ACCOUNT_ENV")
      --email-sender-type string                       The messenger type used to send invitations to new dashboard users. Options: [DRY_RUN AWS_EMAIL] (EMAIL_SENDER_TYPE)
  -h, --help                                           help for add-tenants
      --network-passphrase string                      The Stellar Network passphrase (NETWORK_PASSPHRASE) (default "Test SDF Network ; September 2015")
      --network-type string                            The Stellar Network type (NETWORK_TYPE) (default "testnet")
      --sdp-ui-base-url string                         The Tenant SDP UI/dashboard Base URL. (SDP_UI_BASE_URL) (default "http://localhost:3000")
`
		assert.Equal(t, expectUsageMessage, out.String())
	})

	t.Run("adds new tenant successfully testnet", func(t *testing.T) {
		tenantName := "unhcr"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "UNHCR"

		out := new(strings.Builder)
		rootCmd := rootCmd()
		rootCmd.AddCommand(AddTenantsCmd())
		rootCmd.SetOut(out)
		rootCmd.SetErr(out)
		rootCmd.SetArgs([]string{"add-tenants", tenantName, userFirstName, userLastName, userEmail, organizationName, "--email-sender-type", "DRY_RUN", "--network-type", "testnet", "--multitenant-db-url", dbt.DSN})
		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := rootCmd.ExecuteContext(ctx)
		require.NoError(t, err)
		assert.Empty(t, out.String())

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, tenantName)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 19)
		assert.Equal(t, "tenant unhcr added successfully", entries[17].Message)
		assert.Contains(t, fmt.Sprintf("tenant ID: %s", tenantID), entries[18].Message)

		// Connecting to the new schema
		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		dataSourceName, err := dbConnectionPool.DSN(ctx)
		require.NoError(t, err)
		u, err := url.Parse(dataSourceName)
		require.NoError(t, err)
		uq := u.Query()
		uq.Set("search_path", schemaName)
		u.RawQuery = uq.Encode()

		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
		require.NoError(t, err)
		defer tenantSchemaConnectionPool.Close()

		expectedTablesAfterMigrationsApplied := []string{
			"assets",
			"auth_migrations",
			"auth_user_mfa_codes",
			"auth_user_password_reset",
			"auth_users",
			"countries",
			"disbursements",
			"sdp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"wallets",
			"wallets_assets",
		}
		tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)
		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5", "XLM:"})
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Demo Wallet", "Vibrant Assist"})
		tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
	})

	t.Run("adds new tenant successfully pubnet", func(t *testing.T) {
		tenantName := "irc"
		userFirstName := "First"
		userLastName := "Last"
		userEmail := "email@email.com"
		organizationName := "UNHCR"

		out := new(strings.Builder)
		rootCmd := rootCmd()
		rootCmd.AddCommand(AddTenantsCmd())
		rootCmd.SetOut(out)
		rootCmd.SetErr(out)
		rootCmd.SetArgs([]string{"add-tenants", tenantName, userFirstName, userLastName, userEmail, organizationName, "--email-sender-type", "DRY_RUN", "--network-type", "pubnet", "--multitenant-db-url", dbt.DSN})
		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := rootCmd.ExecuteContext(ctx)
		require.NoError(t, err)
		assert.Empty(t, out.String())

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, tenantName)
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 19)
		assert.Equal(t, "tenant irc added successfully", entries[17].Message)
		assert.Contains(t, fmt.Sprintf("tenant ID: %s", tenantID), entries[18].Message)

		// Connecting to the new schema
		schemaName := fmt.Sprintf("sdp_%s", tenantName)
		dataSourceName, err := dbConnectionPool.DSN(ctx)
		require.NoError(t, err)
		u, err := url.Parse(dataSourceName)
		require.NoError(t, err)
		uq := u.Query()
		uq.Set("search_path", schemaName)
		u.RawQuery = uq.Encode()

		tenantSchemaConnectionPool, err := db.OpenDBConnectionPool(u.String())
		require.NoError(t, err)
		defer tenantSchemaConnectionPool.Close()

		expectedTablesAfterMigrationsApplied := []string{
			"assets",
			"auth_migrations",
			"auth_user_mfa_codes",
			"auth_user_password_reset",
			"auth_users",
			"countries",
			"disbursements",
			"sdp_migrations",
			"messages",
			"organizations",
			"payments",
			"receiver_verifications",
			"receiver_wallets",
			"receivers",
			"wallets",
			"wallets_assets",
		}
		tenant.TenantSchemaMatchTablesFixture(t, ctx, tenantSchemaConnectionPool, schemaName, expectedTablesAfterMigrationsApplied)
		tenant.AssertRegisteredAssetsFixture(t, ctx, tenantSchemaConnectionPool, []string{"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", "XLM:"})
		tenant.AssertRegisteredWalletsFixture(t, ctx, tenantSchemaConnectionPool, []string{"Freedom Wallet", "Vibrant Assist RC", "Vibrant Assist"})
		tenant.AssertRegisteredUserFixture(t, ctx, tenantSchemaConnectionPool, userFirstName, userLastName, userEmail)
	})
}
