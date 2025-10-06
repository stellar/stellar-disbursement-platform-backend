package wallet

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewInMemorySessionCache(t *testing.T) {
	cache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)
	require.NotNil(t, cache)
	assert.NotNil(t, cache.cache)
}

func Test_InMemorySessionCache_Store(t *testing.T) {
	cache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)

	session := webauthn.SessionData{
		Challenge: "test-challenge",
		UserID:    []byte("user-123"),
	}

	err := cache.Store("test-key", SessionTypeRegistration, session, 5*time.Minute)
	require.NoError(t, err)

	retrieved, err := cache.Get("test-key", SessionTypeRegistration)
	require.NoError(t, err)
	assert.Equal(t, session.Challenge, retrieved.Challenge)
	assert.Equal(t, session.UserID, retrieved.UserID)
}

func Test_InMemorySessionCache_Get(t *testing.T) {
	t.Run("returns ErrSessionNotFound if key does not exist", func(t *testing.T) {
		cache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)

		_, err := cache.Get("non-existent-key", SessionTypeRegistration)
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	t.Run("returns ErrSessionTypeMismatch if session type does not match", func(t *testing.T) {
		cache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)

		session := webauthn.SessionData{
			Challenge: "test-challenge",
		}
		err := cache.Store("test-key", SessionTypeRegistration, session, 5*time.Minute)
		require.NoError(t, err)

		_, err = cache.Get("test-key", SessionTypeAuthentication)
		assert.ErrorIs(t, err, ErrSessionTypeMismatch)
	})

	t.Run("returns ErrSessionNotFound if session is expired", func(t *testing.T) {
		cache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)

		session := webauthn.SessionData{
			Challenge: "test-challenge",
		}
		err := cache.Store("test-key", SessionTypeRegistration, session, 1*time.Nanosecond)
		require.NoError(t, err)

		time.Sleep(2 * time.Millisecond)

		_, err = cache.Get("test-key", SessionTypeRegistration)
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	t.Run("successfully retrieves a valid session", func(t *testing.T) {
		cache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)

		session := webauthn.SessionData{
			Challenge: "test-challenge-2",
			UserID:    []byte("user-456"),
		}
		err := cache.Store("test-key-2", SessionTypeAuthentication, session, 5*time.Minute)
		require.NoError(t, err)

		retrieved, err := cache.Get("test-key-2", SessionTypeAuthentication)
		require.NoError(t, err)
		assert.Equal(t, session.Challenge, retrieved.Challenge)
		assert.Equal(t, session.UserID, retrieved.UserID)
	})
}

func Test_InMemorySessionCache_Delete(t *testing.T) {
	t.Run("successfully deletes a session", func(t *testing.T) {
		cache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)

		session := webauthn.SessionData{
			Challenge: "test-challenge",
		}
		err := cache.Store("test-key", SessionTypeRegistration, session, 5*time.Minute)
		require.NoError(t, err)

		_, err = cache.Get("test-key", SessionTypeRegistration)
		require.NoError(t, err)

		cache.Delete("test-key")

		_, err = cache.Get("test-key", SessionTypeRegistration)
		assert.ErrorIs(t, err, ErrSessionNotFound)
	})

	t.Run("does not error when deleting non-existent key", func(t *testing.T) {
		cache := NewInMemorySessionCache(5*time.Minute, 10*time.Minute)

		assert.NotPanics(t, func() {
			cache.Delete("non-existent-key")
		})
	})
}
