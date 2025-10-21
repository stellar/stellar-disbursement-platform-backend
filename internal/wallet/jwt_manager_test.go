package wallet

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testPrivateKey = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgaWqFzmxoHbYUbZEm
EO5XNy9QX3cTAh2jtEi+lOJsnEihRANCAAQ0VOBzsDLy4rqNM5G/Go6IBrRIV7Er
Aftohtbum9ABi8CEq05EzjTGf/D8pzW5RXOhgQhm3jGVv4/fzAtTtunR
-----END PRIVATE KEY-----`

func Test_NewWalletJWTManager(t *testing.T) {
	t.Run("returns error when EC private key is invalid", func(t *testing.T) {
		jwtManager, err := NewWalletJWTManager("invalid")

		assert.EqualError(t, err, "parsing EC private key: invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key")
		assert.Nil(t, jwtManager)
	})

	t.Run("returns error when EC private key is empty", func(t *testing.T) {
		jwtManager, err := NewWalletJWTManager("")

		assert.EqualError(t, err, "parsing EC private key: invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key")
		assert.Nil(t, jwtManager)
	})

	t.Run("successfully creates wallet JWT manager", func(t *testing.T) {
		jwtManager, err := NewWalletJWTManager(testPrivateKey)

		require.NoError(t, err)
		require.NotNil(t, jwtManager)

		manager, ok := jwtManager.(*defaultWalletJWTManager)
		require.True(t, ok)
		require.NotNil(t, manager.privateKey)
	})
}

func Test_WalletJWTManager_GenerateToken(t *testing.T) {
	jwtManager, err := NewWalletJWTManager(testPrivateKey)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("generates token correctly", func(t *testing.T) {
		credentialID := "test-credential-id"
		contractAddress := "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"
		expiresAt := time.Now().Add(time.Hour)

		token, err := jwtManager.GenerateToken(ctx, credentialID, contractAddress, expiresAt)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		gotCredentialID, gotContractAddress, err := jwtManager.ValidateToken(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, credentialID, gotCredentialID)
		assert.Equal(t, contractAddress, gotContractAddress)
	})

	t.Run("generates different tokens for different contract addresses", func(t *testing.T) {
		credentialID := "test-credential-id"
		expiresAt := time.Now().Add(time.Hour)

		token1, err := jwtManager.GenerateToken(ctx, credentialID, "contract1", expiresAt)
		require.NoError(t, err)

		token2, err := jwtManager.GenerateToken(ctx, credentialID, "contract2", expiresAt)
		require.NoError(t, err)

		assert.NotEqual(t, token1, token2)
	})

	t.Run("generates different tokens at different times", func(t *testing.T) {
		credentialID := "test-credential-id"
		contractAddress := "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"
		expiresAt := time.Now().Add(time.Hour)

		token1, err := jwtManager.GenerateToken(ctx, credentialID, contractAddress, expiresAt)
		require.NoError(t, err)

		time.Sleep(time.Millisecond * 10)

		token2, err := jwtManager.GenerateToken(ctx, credentialID, contractAddress, expiresAt)
		require.NoError(t, err)

		assert.NotEqual(t, token1, token2)
	})
}

func Test_WalletJWTManager_ValidateToken(t *testing.T) {
	jwtManager, err := NewWalletJWTManager(testPrivateKey)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("returns error when token has invalid signature", func(t *testing.T) {
		invalidSignatureToken := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJDRExaRkMzU1lKWURaVDdLNjdWWjc1SFBKVklFVVZOSVhGNDdaRzJGQjJSTVFRVlUySEhHQ1lTQyIsImV4cCI6MTY3NTk2Mjk0NywiaWF0IjoxNjc1OTU5MzQ3fQ.invalid_signature_here"

		credentialID, contractAddress, err := jwtManager.ValidateToken(ctx, invalidSignatureToken)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidWalletToken)
		assert.Empty(t, credentialID)
		assert.Empty(t, contractAddress)
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		credentialID := "test-credential-id"
		contractAddress := "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"
		expiresAt := time.Now().Add(-time.Hour)

		token, err := jwtManager.GenerateToken(ctx, credentialID, contractAddress, expiresAt)
		require.NoError(t, err)

		gotCredentialID, gotContractAddress, err := jwtManager.ValidateToken(ctx, token)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrExpiredWalletToken)
		assert.Empty(t, gotCredentialID)
		assert.Empty(t, gotContractAddress)
	})

	t.Run("returns error when token has invalid segments", func(t *testing.T) {
		credentialID, contractAddress, err := jwtManager.ValidateToken(ctx, "token")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidWalletToken)
		assert.Empty(t, credentialID)
		assert.Empty(t, contractAddress)
	})

	t.Run("returns error when token has empty credential_id claim", func(t *testing.T) {
		expiresAt := time.Now().Add(time.Hour)

		token, err := jwtManager.GenerateToken(ctx, "", "", expiresAt)
		require.NoError(t, err)

		credentialID, contractAddress, err := jwtManager.ValidateToken(ctx, token)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingSubClaim)
		assert.Empty(t, credentialID)
		assert.Empty(t, contractAddress)
	})

	t.Run("validates token successfully", func(t *testing.T) {
		credentialID := "test-credential-id"
		contractAddress := "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"
		expiresAt := time.Now().Add(time.Hour)

		token, err := jwtManager.GenerateToken(ctx, credentialID, contractAddress, expiresAt)
		require.NoError(t, err)

		gotCredentialID, gotContractAddress, err := jwtManager.ValidateToken(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, credentialID, gotCredentialID)
		assert.Equal(t, contractAddress, gotContractAddress)
	})

	t.Run("validates token with empty contract_address successfully", func(t *testing.T) {
		credentialID := "test-credential-id"
		expiresAt := time.Now().Add(time.Hour)

		token, err := jwtManager.GenerateToken(ctx, credentialID, "", expiresAt)
		require.NoError(t, err)

		gotCredentialID, gotContractAddress, err := jwtManager.ValidateToken(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, credentialID, gotCredentialID)
		assert.Empty(t, gotContractAddress)
	})
}
