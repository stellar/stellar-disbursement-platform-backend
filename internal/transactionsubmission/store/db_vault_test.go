package store

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/require"
)

// dbVaultAll is a test helper that returns all the dbVaultEntries from the DB.
func dbVaultAll(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) []*DBVaultEntry {
	t.Helper()

	var dbVaultEntries []*DBVaultEntry
	err := dbConnectionPool.SelectContext(ctx, &dbVaultEntries, "SELECT * FROM vault")
	require.NoError(t, err)
	return dbVaultEntries
}

var (
	// dbVaultEntries encrypted with the passphrase "SBLZWEVGNZW4EZVQTKKLQX46WWMETJPNQO5HD2WMIGFONNGG4ZYONGDS"
	dbVaultEntry1 = &DBVaultEntry{PublicKey: "GDSBW3RDJ3H6V3QPKI4YJD4QEOO2SR4FYJHV3JSLE2BWA2RSGLKE3NPO", EncryptedPrivateKey: "5CXbLHEFmH696kgHv1obFurnCr+GvGSAap5kiiYKwG6Ndpnl26TCia49rVM0GtaVtSCUQqwHMlG4LhpsD0atn6BgV/WSRWUphrCG3b+vF+vJ8WnC"}
	dbVaultEntry2 = &DBVaultEntry{PublicKey: "GAIF2YDAESNBDTKGB6FVLQMREVAYUMZSIQIQZBJ72OBXUEQRM263PSFK", EncryptedPrivateKey: "l8fxfRA5TY9QArsHGUzBWTmARNFM+MjP3nMyOiz0JPgo3iGGUP+FCN2TIihjFuB9FM+61DhtsnqL34ZB84b0iX/E1FYp1jwqk6LTWWdS5kzMRIJl"}
)

func Test_DBVaultModel_BatchInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	dbVaultModel := NewDBVaultModel(dbConnectionPool)

	testCases := []struct {
		name            string
		dbVaultEntries  []*DBVaultEntry
		wantErrContains string
		wantFinalCount  int
	}{
		{
			name:           "empty dbVaultEntries wont throw an error",
			dbVaultEntries: nil,
		},
		{
			name: "returns an error if any of the incoming dbVaultEntries has an empty public key",
			dbVaultEntries: []*DBVaultEntry{
				{PublicKey: "", EncryptedPrivateKey: "encrypted-value"},
			},
			wantErrContains: "public key cannot be empty",
		},
		{
			name: "returns an error if any of the incoming dbVaultEntries has an empty encrypted private key",
			dbVaultEntries: []*DBVaultEntry{
				{PublicKey: "GDSBW3RDJ3H6V3QPKI4YJD4QEOO2SR4FYJHV3JSLE2BWA2RSGLKE3NPO", EncryptedPrivateKey: ""},
			},
			wantErrContains: "private key cannot be empty",
		},
		{
			name: "🎉 successfully inserts dbVaultEntries and validate the inserted values match the expected",
			dbVaultEntries: []*DBVaultEntry{
				{PublicKey: "GDSBW3RDJ3H6V3QPKI4YJD4QEOO2SR4FYJHV3JSLE2BWA2RSGLKE3NPO", EncryptedPrivateKey: "5CXbLHEFmH696kgHv1obFurnCr+GvGSAap5kiiYKwG6Ndpnl26TCia49rVM0GtaVtSCUQqwHMlG4LhpsD0atn6BgV/WSRWUphrCG3b+vF+vJ8WnC"},
				{PublicKey: "GAIF2YDAESNBDTKGB6FVLQMREVAYUMZSIQIQZBJ72OBXUEQRM263PSFK", EncryptedPrivateKey: "l8fxfRA5TY9QArsHGUzBWTmARNFM+MjP3nMyOiz0JPgo3iGGUP+FCN2TIihjFuB9FM+61DhtsnqL34ZB84b0iX/E1FYp1jwqk6LTWWdS5kzMRIJl"},
			},
			wantFinalCount: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			defer DeleteAllFromDBVaultEntries(t, ctx, dbConnectionPool)

			dbVaultEntries := dbVaultAll(t, ctx, dbConnectionPool)
			require.Len(t, dbVaultEntries, 0, "this test should have started with 0 distribution accounts")

			err := dbVaultModel.BatchInsert(ctx, tc.dbVaultEntries)

			if tc.wantErrContains == "" {
				require.NoError(t, err)

				dbVaultEntries = dbVaultAll(t, ctx, dbConnectionPool)
				require.Len(t, dbVaultEntries, tc.wantFinalCount)

				// check if dbVaultEntries contains exactly the same elements as tc.dbVaultEntries, order doesn't matter and the only fields that matter are PublicKey and EncryptedPrivateKey
				areSlicesEqual := slices.EqualFunc(tc.dbVaultEntries, dbVaultEntries, func(testVaultEntry, dbVaultEntry *DBVaultEntry) bool {
					return testVaultEntry.PublicKey == dbVaultEntry.PublicKey && testVaultEntry.EncryptedPrivateKey == dbVaultEntry.EncryptedPrivateKey
				})
				require.Truef(t, areSlicesEqual, "the intended and inserted %T slices are not equal", tc.dbVaultEntries)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
				dbVaultEntries = dbVaultAll(t, ctx, dbConnectionPool)
				require.Len(t, dbVaultEntries, 0)
			}
		})
	}
}

func Test_DBVaultModel_Get(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	dbVaultModel := NewDBVaultModel(dbConnectionPool)
	ctx := context.Background()

	// Ensure there are no dbVaultEntries in the DB
	dbVaultEntries := dbVaultAll(t, ctx, dbConnectionPool)
	require.Len(t, dbVaultEntries, 0, "this test should have started with 0 distribution accounts")

	// Insert a dbVaultEntry
	err = dbVaultModel.BatchInsert(ctx, []*DBVaultEntry{dbVaultEntry1, dbVaultEntry2})
	require.NoError(t, err)

	// Assert that the total number of dbVaultEntries is 2
	dbVaultEntries = dbVaultAll(t, ctx, dbConnectionPool)
	require.Len(t, dbVaultEntries, 2)

	// Assert dbVaultEntry1 is returned:
	gotDBVaultEntry, err := dbVaultModel.Get(ctx, dbVaultEntry1.PublicKey)
	require.NoError(t, err)
	require.Equal(t, dbVaultEntry1.PublicKey, gotDBVaultEntry.PublicKey)
	require.Equal(t, dbVaultEntry1.EncryptedPrivateKey, gotDBVaultEntry.EncryptedPrivateKey)
	require.NotEmpty(t, gotDBVaultEntry.CreatedAt)
	require.NotEmpty(t, gotDBVaultEntry.UpdatedAt)

	// Assert dbVaultEntry2 is returned:
	gotDBVaultEntry, err = dbVaultModel.Get(ctx, dbVaultEntry2.PublicKey)
	require.NoError(t, err)
	require.Equal(t, dbVaultEntry2.PublicKey, gotDBVaultEntry.PublicKey)
	require.Equal(t, dbVaultEntry2.EncryptedPrivateKey, gotDBVaultEntry.EncryptedPrivateKey)
	require.NotEmpty(t, gotDBVaultEntry.CreatedAt)
	require.NotEmpty(t, gotDBVaultEntry.UpdatedAt)
}

func Test_DBVaultModel_Delete(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	dbVaultModel := NewDBVaultModel(dbConnectionPool)

	testCases := []struct {
		name                string
		dbVaultToInsert     *DBVaultEntry
		dbVaultToDelete     *DBVaultEntry
		expectedErrorFormat string
	}{
		{
			name:            "🎉 successfully add & delete dbVaultEntry",
			dbVaultToInsert: dbVaultEntry1,
			dbVaultToDelete: dbVaultEntry1,
		},
		{
			name:                "returns an error when trying to delete a dbVaultEntry that does not exist",
			dbVaultToInsert:     dbVaultEntry1,
			dbVaultToDelete:     dbVaultEntry2,
			expectedErrorFormat: "could not find nor delete dbVaultEntry %q: record not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer DeleteAllFromDBVaultEntries(t, ctx, dbConnectionPool)
			err = dbVaultModel.BatchInsert(ctx, []*DBVaultEntry{tc.dbVaultToInsert})
			require.NoError(t, err)

			err = dbVaultModel.Delete(ctx, tc.dbVaultToDelete.PublicKey)
			if tc.expectedErrorFormat != "" {
				require.Error(t, err)
				require.EqualError(t, fmt.Errorf(tc.expectedErrorFormat, tc.dbVaultToDelete.PublicKey), err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
