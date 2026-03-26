package wallet

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
)

func Test_SessionCache_Store_Get(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	cache, err := NewSessionCache(dbConnectionPool, 5*time.Minute)
	require.NoError(t, err)

	ctx := context.Background()
	session := webauthn.SessionData{
		Challenge: "test-challenge",
		UserID:    []byte("user-123"),
	}

	err = cache.Store(ctx, session.Challenge, SessionTypeRegistration, session)
	require.NoError(t, err)

	retrieved, err := cache.Get(ctx, session.Challenge, SessionTypeRegistration)
	require.NoError(t, err)
	assert.Equal(t, session.Challenge, retrieved.Challenge)
	assert.Equal(t, session.UserID, retrieved.UserID)
}

func Test_SessionCache_Get_TypeMismatch(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	cache, err := NewSessionCache(dbConnectionPool, 5*time.Minute)
	require.NoError(t, err)

	ctx := context.Background()
	session := webauthn.SessionData{Challenge: "test-challenge"}
	require.NoError(t, cache.Store(ctx, session.Challenge, SessionTypeRegistration, session))

	_, err = cache.Get(ctx, session.Challenge, SessionTypeAuthentication)
	assert.ErrorIs(t, err, ErrSessionTypeMismatch)
}

func Test_SessionCache_Get_Expired(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	cache, err := NewSessionCache(dbConnectionPool, 5*time.Minute)
	require.NoError(t, err)

	ctx := context.Background()
	challenge := "expired-challenge"
	session := webauthn.SessionData{Challenge: challenge}
	sessionBytes, err := json.Marshal(session)
	require.NoError(t, err)

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	model := data.NewPasskeySessionModel(dbConnectionPool)
	require.NoError(t, model.Store(ctx, challenge, string(SessionTypeRegistration), sessionBytes, expiredAt))

	_, err = cache.Get(ctx, challenge, SessionTypeRegistration)
	assert.ErrorIs(t, err, ErrSessionNotFound)

	var count int
	err = dbConnectionPool.GetContext(ctx, &count, "SELECT COUNT(1) FROM passkey_sessions WHERE challenge = $1", challenge)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func Test_SessionCache_Delete(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	cache, err := NewSessionCache(dbConnectionPool, 5*time.Minute)
	require.NoError(t, err)

	ctx := context.Background()
	session := webauthn.SessionData{Challenge: "delete-challenge"}
	require.NoError(t, cache.Store(ctx, session.Challenge, SessionTypeRegistration, session))

	cache.Delete(ctx, session.Challenge)

	var count int
	err = dbConnectionPool.GetContext(ctx, &count, "SELECT COUNT(1) FROM passkey_sessions WHERE challenge = $1", session.Challenge)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
