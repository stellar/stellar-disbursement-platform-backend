package utils

import (
	"context"
	"math/rand"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db/dbtest"
	"github.com/stretchr/testify/require"
)

func TestAdvisoryLockAndRelease(t *testing.T) {
	ctx := context.Background()
	// Creates a test database:
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	// Creates a database pool
	lockKey := rand.Intn(100000)

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
