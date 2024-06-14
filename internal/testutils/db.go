package testutils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
)

func BeginTxWithRollback(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool) db.DBTransaction {
	t.Helper()

	return beginTx(t, ctx, dbConnectionPool, true)
}

func beginTx(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, autoRollback bool) db.DBTransaction {
	t.Helper()

	dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
	require.NoError(t, err)

	if autoRollback {
		t.Cleanup(func() {
			rollback(t, dbTx)
		})
	}
	return dbTx
}

func rollback(t *testing.T, dbTx db.DBTransaction) {
	t.Helper()

	err := dbTx.Rollback()
	require.NoError(t, err)
}
