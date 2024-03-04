package router

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/require"
)

func Test_GetDNSForAdmin(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	t.Run("a raw DSN will NOT be updated with a search_path", func(t *testing.T) {
		// Checks that the search_path is not set.
		require.NotContains(t, dbt.DSN, "search_path")

		// Sets the search_path to tss.
		updatedDSN, err := GetDNSForAdmin(dbt.DSN)
		require.NoError(t, err)
		t.Log(updatedDSN)
		require.NotContains(t, updatedDSN, "search_path")
	})

	t.Run("a DSN with a search_path will get it removed and cbecome a raw DSN", func(t *testing.T) {
		// Checks that the search_path is not set.
		tssDSN, err := GetDNSForTSS(dbt.DSN)
		require.NoError(t, err)
		require.Contains(t, tssDSN, "search_path="+TSSSchemaName)

		// Sets the search_path to tss.
		updatedDSN, err := GetDNSForAdmin(dbt.DSN)
		require.NoError(t, err)
		t.Log(updatedDSN)
		require.NotContains(t, updatedDSN, "search_path")
	})
}

func Test_GetDNSForTSS(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	// Checks that the search_path is not set.
	require.NotContains(t, dbt.DSN, "search_path="+TSSSchemaName)

	// Sets the search_path to tss.
	updatedDSN, err := GetDNSForTSS(dbt.DSN)
	require.NoError(t, err)
	t.Log(updatedDSN)
	require.Contains(t, updatedDSN, "search_path="+TSSSchemaName)
}

func Test_GetDSNForTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	// Checks that the search_path is not set.
	require.NotContains(t, dbt.DSN, "search_path="+SDPSchemaNamePrefix+"test")

	// Sets the search_path to tss.
	updatedDSN, err := GetDSNForTenant(dbt.DSN, "test")
	require.NoError(t, err)
	t.Log(updatedDSN)
	require.Contains(t, updatedDSN, "search_path="+SDPSchemaNamePrefix+"test")
}
