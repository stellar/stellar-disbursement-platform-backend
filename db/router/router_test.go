package router

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_GetDSNForAdmin(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	// Checks that the search_path is not set.
	require.NotContains(t, dbt.DSN, "search_path="+AdminSchemaName)

	// Sets the search_path to admin.
	updatedDSN, err := GetDSNForAdmin(dbt.DSN)
	require.NoError(t, err)
	t.Log(updatedDSN)
	require.Contains(t, updatedDSN, "search_path="+AdminSchemaName)
}

func Test_GetDSNForTSS(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	// Checks that the search_path is not set.
	require.NotContains(t, dbt.DSN, "search_path="+TSSSchemaName)

	// Sets the search_path to tss.
	updatedDSN, err := GetDSNForTSS(dbt.DSN)
	require.NoError(t, err)
	t.Log(updatedDSN)
	require.Contains(t, updatedDSN, "search_path="+TSSSchemaName)
}

func Test_GetDSNForTenant(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	// Checks that the search_path is not set.
	require.NotContains(t, dbt.DSN, "search_path="+SDPSchemaNamePrefix+"test")

	// Sets the search_path to sdp_test.
	updatedDSN, err := GetDSNForTenant(dbt.DSN, "test")
	require.NoError(t, err)
	t.Log(updatedDSN)
	require.Contains(t, updatedDSN, "search_path="+SDPSchemaNamePrefix+"test")
}

func Test_getDSNWithFixedSchema(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	// Checks that the search_path is not set.
	require.NotContains(t, dbt.DSN, "search_path=test")

	// Sets the search_path to test.
	updatedDSN, err := getDSNWithFixedSchema(dbt.DSN, "test")
	require.NoError(t, err)
	t.Log(updatedDSN)
	require.Contains(t, updatedDSN, "search_path=test")
}

func Test_preExistingSchemasGetOverwritten(t *testing.T) {
	dsnWithoutSearchPath := "postgres://user:password@somehost:5432/test"
	dsnWithSearchPath := "postgres://user:password@somehost:5432/test?search_path=old"

	testCases := []struct {
		name           string
		initialDSN     string
		expectedSchema string
	}{
		{
			name:           "Set search_path to foobar",
			initialDSN:     dsnWithoutSearchPath,
			expectedSchema: "foobar",
		},
		{
			name:           "Set search_path to new",
			initialDSN:     dsnWithSearchPath,
			expectedSchema: "new",
		},
		{
			name:           "GetDSNForAdmin",
			initialDSN:     dsnWithSearchPath,
			expectedSchema: AdminSchemaName,
		},
		{
			name:           "GetDSNForTSS",
			initialDSN:     dsnWithSearchPath,
			expectedSchema: TSSSchemaName,
		},
		{
			name:           "GetDSNForTenant",
			initialDSN:     dsnWithSearchPath,
			expectedSchema: SDPSchemaNamePrefix + "test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			updatedDSN, err := getDSNWithFixedSchema(tc.initialDSN, tc.expectedSchema)
			require.NoError(t, err)
			require.Contains(t, updatedDSN, "search_path="+tc.expectedSchema)
			require.Equal(t, 1, strings.Count(updatedDSN, "search_path"), "search_path should only appear once in the DSN")
		})
	}
}
