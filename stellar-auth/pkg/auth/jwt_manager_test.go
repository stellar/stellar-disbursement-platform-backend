package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NEVER use these values in production!
var (
	testPrivateKey = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgaWqFzmxoHbYUbZEm
EO5XNy9QX3cTAh2jtEi+lOJsnEihRANCAAQ0VOBzsDLy4rqNM5G/Go6IBrRIV7Er
Aftohtbum9ABi8CEq05EzjTGf/D8pzW5RXOhgQhm3jGVv4/fzAtTtunR
-----END PRIVATE KEY-----`
	testPublicKey = `-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAENFTgc7Ay8uK6jTORvxqOiAa0SFex
KwH7aIbW7pvQAYvAhKtORM40xn/w/Kc1uUVzoYEIZt4xlb+P38wLU7bp0Q==
-----END PUBLIC KEY-----`
)

func Test_DefaultJWTManager_GenerateToken(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error when the EC Private Key is invalid", func(t *testing.T) {
		jwtManager := newDefaultJWTManager(withECKeypair(testPublicKey, "invalid"))

		expiresAt := time.Now().Add(time.Minute * 5)
		token, err := jwtManager.GenerateToken(ctx, &User{}, expiresAt)

		assert.EqualError(t, err, "parsing EC Private Key: invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key")
		assert.Empty(t, token)
	})

	t.Run("generates token correctly", func(t *testing.T) {
		jwtManager := newDefaultJWTManager(withECKeypair(testPublicKey, testPrivateKey))

		expiresAt := time.Now().Add(time.Minute * 5)
		token, err := jwtManager.GenerateToken(ctx, &User{}, expiresAt)
		require.NoError(t, err)

		assert.NotEmpty(t, token)
	})
}

func Test_DefaultJWTManager_ValidateToken(t *testing.T) {
	jwtManager := newDefaultJWTManager(withECKeypair(testPublicKey, testPrivateKey))

	ctx := context.Background()

	t.Run("returns false when token has a invalid signature", func(t *testing.T) {
		invalidSignatureToken := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjp7ImlkIjoidXNlci1pZCIsImVtYWlsIjoiZW1haWxAZW1haWwuY29tIiwicm9sZXMiOlt7Im5hbWUiOiJTdXBlcnZpc29yIn1dfSwiZXhwIjoxNjc1OTYyOTQ3fQ.zK9Jb5EMl5rOTOO18SM-q_WOtD0TbL0f9cFfilW9tWHa_vjVMEaf6xRjold9dTPLICDBrqdw_luhKlT370EAiA"

		isValid, err := jwtManager.ValidateToken(ctx, invalidSignatureToken)
		require.NoError(t, err)

		assert.False(t, isValid)
	})

	t.Run("returns false when token is expired", func(t *testing.T) {
		expiresAt := time.Now().Add(time.Minute * -5)
		token, err := jwtManager.GenerateToken(ctx, &User{}, expiresAt)
		require.NoError(t, err)

		isValid, err := jwtManager.ValidateToken(ctx, token)
		require.NoError(t, err)

		assert.False(t, isValid)
	})

	t.Run("returns false when token has invalid segments", func(t *testing.T) {
		isValid, err := jwtManager.ValidateToken(ctx, "token")
		require.NoError(t, err)

		assert.False(t, isValid)
	})

	t.Run("returns true when token is valid", func(t *testing.T) {
		expiresAt := time.Now().Add(time.Minute * 5)
		token, err := jwtManager.GenerateToken(ctx, &User{}, expiresAt)
		require.NoError(t, err)

		isValid, err := jwtManager.ValidateToken(ctx, token)
		require.NoError(t, err)

		assert.True(t, isValid)
	})
}

func Test_DefaultJWTManager_RefreshToken(t *testing.T) {
	jwtManager := newDefaultJWTManager(withECKeypair(testPublicKey, testPrivateKey))

	ctx := context.Background()

	t.Run("returns the same token when is above the refresh period", func(t *testing.T) {
		expiresAt := time.Now().Add(time.Minute * (defaultRefreshTimeoutInMinutes + 1))
		token, err := jwtManager.GenerateToken(ctx, &User{}, expiresAt)
		require.NoError(t, err)

		newExpiresAt := time.Now().Add(time.Minute * 5)
		refreshedToken, err := jwtManager.RefreshToken(ctx, token, newExpiresAt)
		require.NoError(t, err)

		assert.Equal(t, token, refreshedToken)
	})

	t.Run("returns a refreshed token", func(t *testing.T) {
		expiresAt := time.Now().Add(time.Minute * defaultRefreshTimeoutInMinutes)
		token, err := jwtManager.GenerateToken(ctx, &User{}, expiresAt)
		require.NoError(t, err)

		newExpiresAt := time.Now().Add(time.Minute * 5)
		refreshedToken, err := jwtManager.RefreshToken(ctx, token, newExpiresAt)
		require.NoError(t, err)

		assert.NotEqual(t, token, refreshedToken)
	})
}

func Test_DefaultJWTManager_parseToken(t *testing.T) {
	ctx := context.Background()

	t.Run("returns error when the EC Public Key is invalid", func(t *testing.T) {
		jwtManager := newDefaultJWTManager(withECKeypair("invalid", testPrivateKey))

		expiresAt := time.Now().Add(time.Minute * 5)
		tokenString, err := jwtManager.GenerateToken(ctx, &User{}, expiresAt)
		require.NoError(t, err)

		token, c, err := jwtManager.parseToken(tokenString)

		assert.EqualError(t, err, "invalid key: parsing EC Public Key: invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key")
		assert.Nil(t, token)
		assert.Nil(t, c)
	})

	t.Run("returns token and claims correctly", func(t *testing.T) {
		jwtManager := newDefaultJWTManager(withECKeypair(testPublicKey, testPrivateKey))

		expectedUser := &User{
			ID:    "user-ID",
			Email: "email@email.com",
			Roles: []string{
				"role1",
			},
		}

		expiresAt := time.Now().Add(time.Minute * 5).Truncate(time.Second)
		tokenString, err := jwtManager.GenerateToken(ctx, expectedUser, expiresAt)
		require.NoError(t, err)

		token, c, err := jwtManager.parseToken(tokenString)
		require.NoError(t, err)

		assert.Equal(t, expectedUser, c.User)
		assert.Equal(t, expiresAt, c.ExpiresAt.Time)
		assert.Equal(t, tokenString, token.Raw)
	})
}

func Test_DefaultJWTManager_GetUserFromToken(t *testing.T) {
	ctx := context.Background()

	jwtManager := newDefaultJWTManager(withECKeypair(testPublicKey, testPrivateKey))

	expectedUser := &User{
		ID:    "user-id",
		Email: "email@email.com",
		Roles: []string{"role1", "role2"},
	}

	expiresAt := time.Now().Add(time.Minute * 5).Truncate(time.Second)
	token, err := jwtManager.GenerateToken(ctx, expectedUser, expiresAt)
	require.NoError(t, err)

	gotUser, err := jwtManager.GetUserFromToken(ctx, token)
	require.NoError(t, err)

	assert.Equal(t, expectedUser, gotUser)
}
