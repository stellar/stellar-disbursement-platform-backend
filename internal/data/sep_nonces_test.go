package data

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
)

func Test_SEPNonceModel_Store(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := NewSEPNonceModel(dbConnectionPool)

	t.Run("successfully stores nonce", func(t *testing.T) {
		nonce := "nonce-store"
		expiresAt := time.Now().UTC().Add(5 * time.Minute)

		err := model.Store(ctx, nonce, expiresAt)
		require.NoError(t, err)

		var storedExpiresAt time.Time
		err = dbConnectionPool.GetContext(ctx, &storedExpiresAt, "SELECT expires_at FROM sep_nonces WHERE nonce = $1", nonce)
		require.NoError(t, err)
		assert.WithinDuration(t, expiresAt, storedExpiresAt, time.Second)
	})

	t.Run("returns error for duplicate nonce", func(t *testing.T) {
		nonce := "nonce-duplicate"
		expiresAt := time.Now().UTC().Add(5 * time.Minute)

		require.NoError(t, model.Store(ctx, nonce, expiresAt))

		err := model.Store(ctx, nonce, expiresAt.Add(time.Minute))
		require.ErrorContains(t, err, "duplicate key value violates unique constraint")

		var count int
		err = dbConnectionPool.GetContext(ctx, &count, "SELECT COUNT(1) FROM sep_nonces WHERE nonce = $1", nonce)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}

func Test_SEPNonceModel_Consume(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := NewSEPNonceModel(dbConnectionPool)

	t.Run("successfully consumes nonce", func(t *testing.T) {
		nonce := "nonce-consume"
		expiresAt := time.Now().UTC().Add(10 * time.Minute)
		require.NoError(t, model.Store(ctx, nonce, expiresAt))

		consumedAt, ok, err := model.Consume(ctx, nonce)
		require.NoError(t, err)
		assert.True(t, ok)
		assert.WithinDuration(t, expiresAt, consumedAt, time.Second)

		var count int
		err = dbConnectionPool.GetContext(ctx, &count, "SELECT COUNT(1) FROM sep_nonces WHERE nonce = $1", nonce)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns false when nonce missing", func(t *testing.T) {
		expiresAt, ok, err := model.Consume(ctx, "missing")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.True(t, expiresAt.IsZero())
	})
}

func Test_SEPNonceModel_DeleteExpired(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := NewSEPNonceModel(dbConnectionPool)

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	activeAt := time.Now().UTC().Add(5 * time.Minute)

	_, err := dbConnectionPool.ExecContext(
		ctx,
		"INSERT INTO sep_nonces (nonce, expires_at) VALUES ($1, $2), ($3, $4)",
		"expired-nonce",
		expiredAt,
		"active-nonce",
		activeAt,
	)
	require.NoError(t, err)

	err = model.DeleteExpired(ctx)
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
