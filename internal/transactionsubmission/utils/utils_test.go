package utils

import (
	"context"
	"net/http"
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/render/problem"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/require"
)

func Test_GetHorizonErrorString(t *testing.T) {
	hError := horizonclient.Error{
		Problem: problem.P{
			Title:  "Transaction Failed",
			Type:   "transaction_failed",
			Status: http.StatusBadRequest,
			Detail: "",
			Extras: map[string]interface{}{
				"result_codes": map[string]interface{}{
					"transaction": "tx_failed",
					"operations":  []string{"op_underfunded"},
				},
			},
		},
	}

	errStr := GetHorizonErrorString(hError)
	wantErrStr := "Type: transaction_failed, Title: Transaction Failed, Status: 400, Detail: , Extras: map[result_codes:map[operations:[op_underfunded] transaction:tx_failed]]"
	require.Equal(t, wantErrStr, errStr)
}

func TestAdvisoryLockAndRelease(t *testing.T) {
	ctx := context.Background()
	// Creates a test database:
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	// Creates a database pool
	lockKey := 123
	dbConnectionPool1, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	lockAcquired, err := AcquireAdvisoryLock(ctx, dbConnectionPool1, lockKey)
	require.NoError(t, err)

	// Should be able to acquire the lock
	require.True(t, lockAcquired)
	require.NoError(t, err)

	// Create another database pool
	dbConnectionPool2, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool2.Close()
	lockAcquired2, err := AcquireAdvisoryLock(ctx, dbConnectionPool2, lockKey)
	require.NoError(t, err)
	// Should not be able to acquire the lock since its already been acquired
	require.False(t, lockAcquired2)

	// Close the original connection which releases the lock
	dbConnectionPool1.Close()

	// try to acquire the lock again
	lockAcquired3, err := AcquireAdvisoryLock(ctx, dbConnectionPool2, lockKey)
	require.NoError(t, err)
	// Should be able to acquire the lock since we called dbConnectionPool1.Close()
	require.True(t, lockAcquired3)
}
