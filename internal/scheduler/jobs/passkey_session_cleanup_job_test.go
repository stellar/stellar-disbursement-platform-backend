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

func Test_PasskeySessionCleanupJob(t *testing.T) {
	j := passkeySessionCleanupJob{}

	assert.Equal(t, passkeySessionCleanupJobName, j.GetName())
	assert.Equal(t, passkeySessionCleanupJobInterval, j.GetInterval())
	assert.True(t, j.IsJobMultiTenant())
}

func Test_PasskeySessionCleanupJob_Execute(t *testing.T) {
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

	sessionBytes := []byte(`{"challenge":"test"}`)

	_, err = dbConnectionPool.ExecContext(
		ctx,
		"INSERT INTO passkey_sessions (challenge, session_type, session_data, expires_at) VALUES ($1, $2, $3, $4), ($5, $6, $7, $8)",
		"expired-session",
		"registration",
		sessionBytes,
		expiredAt,
		"active-session",
		"registration",
		sessionBytes,
		activeAt,
	)
	require.NoError(t, err)

	job := NewPasskeySessionCleanupJob(models)
	err = job.Execute(ctx)
	require.NoError(t, err)

	var expiredCount int
	err = dbConnectionPool.GetContext(ctx, &expiredCount, "SELECT COUNT(1) FROM passkey_sessions WHERE challenge = $1", "expired-session")
	require.NoError(t, err)
	assert.Equal(t, 0, expiredCount)

	var activeCount int
	err = dbConnectionPool.GetContext(ctx, &activeCount, "SELECT COUNT(1) FROM passkey_sessions WHERE challenge = $1", "active-session")
	require.NoError(t, err)
	assert.Equal(t, 1, activeCount)
}

func Test_NewPasskeySessionCleanupJob(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	job := NewPasskeySessionCleanupJob(models)
	require.NotNil(t, job)
}
