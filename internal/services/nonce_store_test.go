package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewInMemoryNonceStore(t *testing.T) {
	testCases := []struct {
		name       string
		expiration time.Duration
		maxEntries int
		wantErr    bool
	}{
		{
			name:       "error when maxEntries is invalid",
			expiration: 5 * time.Minute,
			maxEntries: 0,
			wantErr:    true,
		},
		{
			name:       "error when defaultExpiration is invalid",
			expiration: 0,
			maxEntries: 10,
			wantErr:    true,
		},
		{
			name:       "success with valid inputs",
			expiration: 5 * time.Minute,
			maxEntries: 10,
			wantErr:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewInMemoryNonceStore(tc.expiration, tc.maxEntries)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, store)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, store)
		})
	}
}

func Test_InMemoryNonceStore_Store(t *testing.T) {
	testCases := []struct {
		name    string
		nonce   string
		wantErr bool
	}{
		{
			name:    "error when nonce is empty",
			nonce:   "",
			wantErr: true,
		},
		{
			name:    "success when nonce is valid",
			nonce:   "nonce-1",
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewInMemoryNonceStore(5*time.Minute, 10)
			require.NoError(t, err)

			err = store.Store(tc.nonce)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func Test_InMemoryNonceStore_Consume(t *testing.T) {
	t.Run("error when nonce is empty", func(t *testing.T) {
		store, err := NewInMemoryNonceStore(5*time.Minute, 10)
		require.NoError(t, err)

		ok, err := store.Consume("")
		assert.Error(t, err)
		assert.False(t, ok)
	})

	t.Run("missing nonce returns false", func(t *testing.T) {
		store, err := NewInMemoryNonceStore(5*time.Minute, 10)
		require.NoError(t, err)

		ok, err := store.Consume("missing")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("expired nonce returns false", func(t *testing.T) {
		store, err := NewInMemoryNonceStore(1*time.Millisecond, 10)
		require.NoError(t, err)

		require.NoError(t, store.Store("nonce-expired"))
		time.Sleep(3 * time.Millisecond)

		ok, err := store.Consume("nonce-expired")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("consumes stored nonce", func(t *testing.T) {
		store, err := NewInMemoryNonceStore(5*time.Minute, 10)
		require.NoError(t, err)

		require.NoError(t, store.Store("nonce-valid"))

		ok, err := store.Consume("nonce-valid")
		require.NoError(t, err)
		assert.True(t, ok)

		ok, err = store.Consume("nonce-valid")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}
