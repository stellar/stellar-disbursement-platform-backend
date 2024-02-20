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

func stellarSignatoryAll(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) []*StellarSignatory {
	t.Helper()

	var stellarSignatories []*StellarSignatory
	err := dbConnectionPool.SelectContext(ctx, &stellarSignatories, "SELECT * FROM stellar_signatories")
	require.NoError(t, err)
	return stellarSignatories
}

var (
	// signatories encrypted with the passphrase "SBLZWEVGNZW4EZVQTKKLQX46WWMETJPNQO5HD2WMIGFONNGG4ZYONGDS"
	signatory1 = &StellarSignatory{PublicKey: "GDSBW3RDJ3H6V3QPKI4YJD4QEOO2SR4FYJHV3JSLE2BWA2RSGLKE3NPO", EncryptedPrivateKey: "5CXbLHEFmH696kgHv1obFurnCr+GvGSAap5kiiYKwG6Ndpnl26TCia49rVM0GtaVtSCUQqwHMlG4LhpsD0atn6BgV/WSRWUphrCG3b+vF+vJ8WnC"}
	signatory2 = &StellarSignatory{PublicKey: "GAIF2YDAESNBDTKGB6FVLQMREVAYUMZSIQIQZBJ72OBXUEQRM263PSFK", EncryptedPrivateKey: "l8fxfRA5TY9QArsHGUzBWTmARNFM+MjP3nMyOiz0JPgo3iGGUP+FCN2TIihjFuB9FM+61DhtsnqL34ZB84b0iX/E1FYp1jwqk6LTWWdS5kzMRIJl"}
)

func Test_StellarSignatoryModel_BatchInsert(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	stellarSignatoryModel := NewStellarSignatoryModel(dbConnectionPool)

	testCases := []struct {
		name               string
		stellarSignatories []*StellarSignatory
		wantErrContains    string
		wantFinalCount     int
	}{
		{
			name:               "empty stellar signatories wont throw an error",
			stellarSignatories: nil,
		},
		{
			name: "returns an error if any of the incoming signatories has an empty public key",
			stellarSignatories: []*StellarSignatory{
				{PublicKey: "", EncryptedPrivateKey: "encrypted-value"},
			},
			wantErrContains: "public key cannot be empty",
		},
		{
			name: "returns an error if any of the incoming signatories has an empty encrypted private key",
			stellarSignatories: []*StellarSignatory{
				{PublicKey: "GDSBW3RDJ3H6V3QPKI4YJD4QEOO2SR4FYJHV3JSLE2BWA2RSGLKE3NPO", EncryptedPrivateKey: ""},
			},
			wantErrContains: "private key cannot be empty",
		},
		{
			name: "🎉 successfully inserts signatories and validate the inserted values match the expected",
			stellarSignatories: []*StellarSignatory{
				{PublicKey: "GDSBW3RDJ3H6V3QPKI4YJD4QEOO2SR4FYJHV3JSLE2BWA2RSGLKE3NPO", EncryptedPrivateKey: "5CXbLHEFmH696kgHv1obFurnCr+GvGSAap5kiiYKwG6Ndpnl26TCia49rVM0GtaVtSCUQqwHMlG4LhpsD0atn6BgV/WSRWUphrCG3b+vF+vJ8WnC"},
				{PublicKey: "GAIF2YDAESNBDTKGB6FVLQMREVAYUMZSIQIQZBJ72OBXUEQRM263PSFK", EncryptedPrivateKey: "l8fxfRA5TY9QArsHGUzBWTmARNFM+MjP3nMyOiz0JPgo3iGGUP+FCN2TIihjFuB9FM+61DhtsnqL34ZB84b0iX/E1FYp1jwqk6LTWWdS5kzMRIJl"},
			},
			wantFinalCount: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			defer DeleteAllFromStellarSignatories(t, ctx, dbConnectionPool)

			stellarSignatories := stellarSignatoryAll(t, ctx, dbConnectionPool)
			require.Len(t, stellarSignatories, 0, "this test should have started with 0 distribution accounts")

			err := stellarSignatoryModel.BatchInsert(ctx, tc.stellarSignatories)

			if tc.wantErrContains == "" {
				require.NoError(t, err)

				stellarSignatories = stellarSignatoryAll(t, ctx, dbConnectionPool)
				require.Len(t, stellarSignatories, tc.wantFinalCount)

				// check if stellarSignatories contains exactly the same elements as tc.stellarSignatories, order doesn't matter and the only fields that matter are PublicKey and EncryptedPrivateKey
				areSlicesEqual := slices.EqualFunc(tc.stellarSignatories, stellarSignatories, func(testSignatory, dbSignatory *StellarSignatory) bool {
					return testSignatory.PublicKey == dbSignatory.PublicKey && testSignatory.EncryptedPrivateKey == dbSignatory.EncryptedPrivateKey
				})
				require.Truef(t, areSlicesEqual, "the intended and inserted %T slices are not equal", tc.stellarSignatories)
			} else {
				require.ErrorContains(t, err, tc.wantErrContains)
				stellarSignatories = stellarSignatoryAll(t, ctx, dbConnectionPool)
				require.Len(t, stellarSignatories, 0)
			}
		})
	}
}

func Test_StellarSignatoryModel_Get(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	stellarSignatoryModel := NewStellarSignatoryModel(dbConnectionPool)
	ctx := context.Background()

	// Ensure there are no stellar signatories in the DB
	stellarSignatories := stellarSignatoryAll(t, ctx, dbConnectionPool)
	require.Len(t, stellarSignatories, 0, "this test should have started with 0 distribution accounts")

	// Insert a stellar signatory
	err = stellarSignatoryModel.BatchInsert(ctx, []*StellarSignatory{signatory1, signatory2})
	require.NoError(t, err)

	// Assert that the total number of signatories is 2
	stellarSignatories = stellarSignatoryAll(t, ctx, dbConnectionPool)
	require.Len(t, stellarSignatories, 2)

	// Assert signatory1 is returned:
	gotStellarSignatory, err := stellarSignatoryModel.Get(ctx, signatory1.PublicKey)
	require.NoError(t, err)
	require.Equal(t, signatory1.PublicKey, gotStellarSignatory.PublicKey)
	require.Equal(t, signatory1.EncryptedPrivateKey, gotStellarSignatory.EncryptedPrivateKey)
	require.NotEmpty(t, gotStellarSignatory.CreatedAt)
	require.NotEmpty(t, gotStellarSignatory.UpdatedAt)

	// Assert signatory2 is returned:
	gotStellarSignatory, err = stellarSignatoryModel.Get(ctx, signatory2.PublicKey)
	require.NoError(t, err)
	require.Equal(t, signatory2.PublicKey, gotStellarSignatory.PublicKey)
	require.Equal(t, signatory2.EncryptedPrivateKey, gotStellarSignatory.EncryptedPrivateKey)
	require.NotEmpty(t, gotStellarSignatory.CreatedAt)
	require.NotEmpty(t, gotStellarSignatory.UpdatedAt)
}

func Test_StellarSignatoryModel_Delete(t *testing.T) {
	dbt := dbtest.OpenWithTSSMigrationsOnly(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	stellarSignatoryModel := NewStellarSignatoryModel(dbConnectionPool)

	testCases := []struct {
		name                     string
		stelarSignatoryToAdd     *StellarSignatory
		stellarSignatoryToDelete *StellarSignatory
		expectedErrorFormat      string
	}{
		{
			name:                     "🎉 successfully add & delete stellar signatory",
			stelarSignatoryToAdd:     signatory1,
			stellarSignatoryToDelete: signatory1,
		},
		{
			name:                     "returns an error when trying to delete a stellar signatory that does not exist",
			stelarSignatoryToAdd:     signatory1,
			stellarSignatoryToDelete: signatory2,
			expectedErrorFormat:      "could not find nor delete signatory %q: record not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer DeleteAllFromStellarSignatories(t, ctx, dbConnectionPool)
			err = stellarSignatoryModel.BatchInsert(ctx, []*StellarSignatory{tc.stelarSignatoryToAdd})
			require.NoError(t, err)

			err = stellarSignatoryModel.Delete(ctx, tc.stellarSignatoryToDelete.PublicKey)
			if tc.expectedErrorFormat != "" {
				require.Error(t, err)
				require.EqualError(t, fmt.Errorf(tc.expectedErrorFormat, tc.stellarSignatoryToDelete.PublicKey), err.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
