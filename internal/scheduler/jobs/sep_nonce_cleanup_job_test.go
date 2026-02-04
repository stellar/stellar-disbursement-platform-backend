package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_SEPNonceCleanupJob(t *testing.T) {
	j := sepNonceCleanupJob{}

	assert.Equal(t, sepNonceCleanupJobName, j.GetName())
	assert.Equal(t, sepNonceCleanupJobInterval, j.GetInterval())
	assert.True(t, j.IsJobMultiTenant())
}

func Test_SEPNonceCleanupJob_Execute(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	activeAt := time.Now().UTC().Add(2 * time.Minute)

	_, err = dbConnectionPool.ExecContext(
		ctx,
		"INSERT INTO sep_nonces (nonce, expires_at) VALUES ($1, $2), ($3, $4)",
		"expired-nonce",
		expiredAt,
		"active-nonce",
		activeAt,
	)
	require.NoError(t, err)

	job := NewSEPNonceCleanupJob(models)
	err = job.Execute(ctx)
	require.NoError(t, err)

	var expiredCount int
	err = dbConnectionPool.GetContext(ctx, &expiredCount, "SELECT COUNT(1) FROM sep_nonces WHERE nonce = $1", "expired-nonce")
	require.NoError(t, err)
	assert.Equal(t, 0, expiredCount)

	var activeCount int
	err = dbConnectionPool.GetContext(ctx, &activeCount, "SELECT COUNT(1) FROM sep_nonces WHERE nonce = $1", "active-nonce")
	require.NoError(t, err)
	assert.Equal(t, 1, activeCount)
}

func Test_NewSEPNonceCleanupJob(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	job := NewSEPNonceCleanupJob(models)
	require.NotNil(t, job)
}
