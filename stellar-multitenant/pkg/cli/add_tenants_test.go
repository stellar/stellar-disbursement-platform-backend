package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func DeleteAllTenantsFixture(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) {
	const q = "DELETE FROM tenants"
	_, err := dbConnectionPool.ExecContext(ctx, q)
	require.NoError(t, err)
}

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

	ctx := context.Background()

	t.Run("adds a new tenant successfully", func(t *testing.T) {
		DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := executeAddTenant(ctx, dbt.DSN, "myorg")
		assert.Nil(t, err)

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, "myorg")
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "tenant myorg added successfully", entries[0].Message)
		assert.Contains(t, fmt.Sprintf("tenant ID: %s", tenantID), entries[1].Message)
	})

	t.Run("duplicated tenant name", func(t *testing.T) {
		DeleteAllTenantsFixture(t, ctx, dbConnectionPool)

		getEntries := log.DefaultLogger.StartTest(log.DebugLevel)

		err := executeAddTenant(ctx, dbt.DSN, "myorg")
		assert.Nil(t, err)

		err = executeAddTenant(ctx, dbt.DSN, "MyOrg")
		assert.ErrorIs(t, err, tenant.ErrDuplicatedTenantName)

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, "myorg")
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 2)
		assert.Equal(t, "tenant myorg added successfully", entries[0].Message)
		assert.Contains(t, fmt.Sprintf("tenant ID: %s", tenantID), entries[1].Message)
	})
}

func Test_AddTenantsCmd(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()

	t.Run("shows usage", func(t *testing.T) {
		out := new(strings.Builder)
		mockCmd := cobra.Command{}
		mockCmd.AddCommand(AddTenantsCmd())
		mockCmd.SetOut(out)
		mockCmd.SetErr(out)
		mockCmd.SetArgs([]string{"add-tenants"})
		err := mockCmd.ExecuteContext(ctx)
		assert.EqualError(t, err, "accepts 1 arg(s), received 0")

		expectUsageMessage := `Error: accepts 1 arg(s), received 0
Usage:
   add-tenants [flags]

Examples:
add-tenants [name]

Flags:
  -h, --help   help for add-tenants

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
add-tenants [name]

Flags:
  -h, --help   help for add-tenants
`
		assert.Equal(t, expectUsageMessage, out.String())
	})

	t.Run("adds new tenant successfully", func(t *testing.T) {
		out := new(strings.Builder)
		rootCmd := rootCmd()
		rootCmd.AddCommand(AddTenantsCmd())
		rootCmd.SetOut(out)
		rootCmd.SetErr(out)
		rootCmd.SetArgs([]string{"add-tenants", "unhcr", "--multitenant-db-url", dbt.DSN})
		getEntries := log.DefaultLogger.StartTest(log.InfoLevel)

		err := rootCmd.ExecuteContext(ctx)
		require.NoError(t, err)
		assert.Empty(t, out.String())

		const q = "SELECT id FROM tenants WHERE name = $1"
		var tenantID string
		err = dbConnectionPool.GetContext(ctx, &tenantID, q, "unhcr")
		require.NoError(t, err)

		entries := getEntries()
		require.Len(t, entries, 4)
		assert.Equal(t, "tenant unhcr added successfully", entries[2].Message)
		assert.Contains(t, fmt.Sprintf("tenant ID: %s", tenantID), entries[3].Message)
	})
}
