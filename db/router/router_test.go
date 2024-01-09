package router

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/require"
)

func Test_GetTSSDatabaseDSN(t *testing.T) {
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
