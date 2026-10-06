package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_EnvelopeEncrypter(t *testing.T) {
	e := EnvelopeEncrypter{}
	const kek = "host-level-distribution-passphrase"
	const message = "SCYV43Y4UQUXLVHZ4PI7NIH4SCSWLHUTRZ3YHDOXIQDF6UF2BCXLDTZK"

	t.Run("round trip", func(t *testing.T) {
		ciphertext, encryptedDEK, err := e.EncryptWithNewDEK(message, kek)
		require.NoError(t, err)
		assert.NotEqual(t, message, ciphertext)

		got, err := e.DecryptWithDEK(ciphertext, encryptedDEK, kek)
		require.NoError(t, err)
		assert.Equal(t, message, got)
	})

	t.Run("each envelope uses a unique DEK", func(t *testing.T) {
		c1, d1, err := e.EncryptWithNewDEK(message, kek)
		require.NoError(t, err)
		c2, d2, err := e.EncryptWithNewDEK(message, kek)
		require.NoError(t, err)

		assert.NotEqual(t, c1, c2, "same plaintext must yield different ciphertext (fresh DEK + nonce)")
		assert.NotEqual(t, d1, d2, "each envelope must wrap a distinct DEK")
	})

	t.Run("wrong KEK fails closed", func(t *testing.T) {
		ciphertext, encryptedDEK, err := e.EncryptWithNewDEK(message, kek)
		require.NoError(t, err)

		_, err = e.DecryptWithDEK(ciphertext, encryptedDEK, "wrong-passphrase")
		require.Error(t, err)
		assert.ErrorContains(t, err, "unwrapping DEK")
	})

	t.Run("truncated ciphertext fails closed with error, not panic", func(t *testing.T) {
		// Regression: utils.Decrypt used to panic on ciphertext shorter than the GCM nonce.
		// "Y29ycnVwdGVk" decodes to 9 bytes — below the 12-byte nonce floor.
		_, err := e.DecryptWithDEK("Y29ycnVwdGVk", "Y29ycnVwdGVk", kek)
		require.Error(t, err)
		assert.ErrorContains(t, err, "too short")
	})

	t.Run("tampered encrypted DEK fails closed (GCM auth)", func(t *testing.T) {
		ciphertext, _, err := e.EncryptWithNewDEK(message, kek)
		require.NoError(t, err)
		_, otherDEK, err := e.EncryptWithNewDEK("other secret", kek)
		require.NoError(t, err)

		// Valid base64, valid KEK unwrap — but the DEK belongs to a different envelope, so
		// decrypting the ciphertext must fail GCM authentication.
		_, err = e.DecryptWithDEK(ciphertext, otherDEK, kek)
		require.Error(t, err)
	})

	t.Run("rotation preserves plaintext under a fresh DEK", func(t *testing.T) {
		ciphertext, encryptedDEK, err := e.EncryptWithNewDEK(message, kek)
		require.NoError(t, err)

		newCiphertext, newEncryptedDEK, err := e.RotateDEK(ciphertext, encryptedDEK, kek)
		require.NoError(t, err)
		assert.NotEqual(t, ciphertext, newCiphertext)
		assert.NotEqual(t, encryptedDEK, newEncryptedDEK)

		got, err := e.DecryptWithDEK(newCiphertext, newEncryptedDEK, kek)
		require.NoError(t, err)
		assert.Equal(t, message, got)

		// The pre-rotation envelope still decrypts too (rotation does not invalidate inputs;
		// persistence replacement is the service's job).
		got, err = e.DecryptWithDEK(ciphertext, encryptedDEK, kek)
		require.NoError(t, err)
		assert.Equal(t, message, got)
	})
}
