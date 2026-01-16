package data

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
)

func Test_PasskeySessionModel_Store_Get_Delete(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := NewPasskeySessionModel(dbConnectionPool)

	session := webauthn.SessionData{
		Challenge: "test-challenge",
		UserID:    []byte("user-123"),
	}
	sessionBytes, err := json.Marshal(session)
	require.NoError(t, err)

	expiresAt := time.Now().UTC().Add(5 * time.Minute)

	err = model.Store(ctx, session.Challenge, "registration", sessionBytes, expiresAt)
	require.NoError(t, err)

	stored, err := model.Get(ctx, session.Challenge)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "registration", stored.SessionType)
	assert.WithinDuration(t, expiresAt, stored.ExpiresAt, time.Second)

	var storedSession webauthn.SessionData
	require.NoError(t, json.Unmarshal(stored.SessionData, &storedSession))
	assert.Equal(t, session.Challenge, storedSession.Challenge)
	assert.Equal(t, session.UserID, storedSession.UserID)

	require.NoError(t, model.Delete(ctx, session.Challenge))
	stored, err = model.Get(ctx, session.Challenge)
	require.NoError(t, err)
	assert.Nil(t, stored)
}

func Test_PasskeySessionModel_Get(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := NewPasskeySessionModel(dbConnectionPool)

	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	session := webauthn.SessionData{
		Challenge: "stored-challenge",
		UserID:    []byte("user-123"),
	}
	sessionBytes, err := json.Marshal(session)
	require.NoError(t, err)

	tests := []struct {
		name        string
		challenge   string
		seed        func(t *testing.T)
		wantNil     bool
		wantType    string
		wantExpires time.Time
		wantSession *webauthn.SessionData
	}{
		{
			name:      "missing",
			challenge: "missing",
			wantNil:   true,
		},
		{
			name:      "stored",
			challenge: session.Challenge,
			seed: func(t *testing.T) {
				require.NoError(t, model.Store(ctx, session.Challenge, "registration", sessionBytes, expiresAt))
			},
			wantType:    "registration",
			wantExpires: expiresAt,
			wantSession: &session,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.seed != nil {
				tt.seed(t)
			}

			stored, err := model.Get(ctx, tt.challenge)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, stored)
				return
			}

			require.NotNil(t, stored)
			assert.Equal(t, tt.wantType, stored.SessionType)
			assert.WithinDuration(t, tt.wantExpires, stored.ExpiresAt, time.Second)

			if tt.wantSession != nil {
				var storedSession webauthn.SessionData
				require.NoError(t, json.Unmarshal(stored.SessionData, &storedSession))
				assert.Equal(t, tt.wantSession.Challenge, storedSession.Challenge)
				assert.Equal(t, tt.wantSession.UserID, storedSession.UserID)
			}
		})
	}
}

func Test_PasskeySessionModel_DeleteExpired(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := NewPasskeySessionModel(dbConnectionPool)

	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	activeAt := time.Now().UTC().Add(5 * time.Minute)

	sessionBytes := []byte(`{"challenge":"test"}`)

	require.NoError(t, model.Store(ctx, "expired", "registration", sessionBytes, expiredAt))
	require.NoError(t, model.Store(ctx, "active", "registration", sessionBytes, activeAt))

	err := model.DeleteExpired(ctx)
	require.NoError(t, err)

	var expiredCount int
	err = dbConnectionPool.GetContext(ctx, &expiredCount, "SELECT COUNT(1) FROM passkey_sessions WHERE challenge = $1", "expired")
	require.NoError(t, err)
	assert.Equal(t, 0, expiredCount)

	var activeCount int
	err = dbConnectionPool.GetContext(ctx, &activeCount, "SELECT COUNT(1) FROM passkey_sessions WHERE challenge = $1", "active")
	require.NoError(t, err)
	assert.Equal(t, 1, activeCount)
}

func Test_PasskeySessionModel_Store_ConflictReturnsError(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, outerErr := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, outerErr)
	defer dbConnectionPool.Close()

	ctx := context.Background()
	model := NewPasskeySessionModel(dbConnectionPool)

	challenge := "conflict-challenge"
	sessionType := "registration"

	expiresAt := time.Now().UTC().Add(10 * time.Minute)
	updatedExpiry := time.Now().UTC().Add(5 * time.Minute)

	require.NoError(t, model.Store(ctx, challenge, sessionType, []byte(`{"challenge":"first"}`), expiresAt))
	err := model.Store(ctx, challenge, sessionType, []byte(`{"challenge":"second"}`), updatedExpiry)
	require.ErrorIs(t, err, ErrRecordAlreadyExists)

	stored, err := model.Get(ctx, challenge)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, sessionType, stored.SessionType)
	assert.WithinDuration(t, expiresAt, stored.ExpiresAt, time.Second)

	var storedSession webauthn.SessionData
	require.NoError(t, json.Unmarshal(stored.SessionData, &storedSession))
	assert.Equal(t, "first", storedSession.Challenge)
}
