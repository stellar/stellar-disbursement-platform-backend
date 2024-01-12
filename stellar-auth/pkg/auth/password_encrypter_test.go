package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DefaultPasswordEncrypter_Encrypt(t *testing.T) {
	passwordEncrypter := NewDefaultPasswordEncrypter()

	ctx := context.Background()

	t.Run("returns err when password is too short", func(t *testing.T) {
		password := ""

		encryptedPassword, err := passwordEncrypter.Encrypt(ctx, password)

		assert.EqualError(t, err, ErrPasswordTooShort.Error())
		assert.Empty(t, encryptedPassword)

		password = "secret"

		encryptedPassword, err = passwordEncrypter.Encrypt(ctx, password)

		assert.EqualError(t, err, ErrPasswordTooShort.Error())
		assert.Empty(t, encryptedPassword)
	})

	t.Run("returns err when password is too long", func(t *testing.T) {
		password := "G635a3LBOtS!vh6hyuvZFlgG@wLuE6IRd3k#rk"

		encryptedPassword, err := passwordEncrypter.Encrypt(ctx, password)

		assert.EqualError(t, err, ErrPasswordTooLong.Error())
		assert.Empty(t, encryptedPassword)
	})

	t.Run("encrypts the password correctly", func(t *testing.T) {
		password := "mysecret1234"

		encryptedPassword, err := passwordEncrypter.Encrypt(ctx, password)
		require.NoError(t, err)

		assert.NotEmpty(t, encryptedPassword)
		assert.NotEqual(t, password, encryptedPassword)
		assert.Len(t, encryptedPassword, 60)

		password = "myanothersecret"

		encryptedPassword, err = passwordEncrypter.Encrypt(ctx, password)
		require.NoError(t, err)

		assert.NotEmpty(t, encryptedPassword)
		assert.NotEqual(t, password, encryptedPassword)
		assert.Len(t, encryptedPassword, 60)
	})
}

func Test_DefaultPasswordEncrypter_ComparePassword(t *testing.T) {
	passwordEncrypter := NewDefaultPasswordEncrypter()

	ctx := context.Background()

	t.Run("returns false when the password is wrong", func(t *testing.T) {
		password := "mysecret1234"

		encryptedPassword, err := passwordEncrypter.Encrypt(ctx, password)
		require.NoError(t, err)

		isEqual, err := passwordEncrypter.ComparePassword(ctx, encryptedPassword, "wrongsecret")
		require.NoError(t, err)

		assert.False(t, isEqual)
	})

	t.Run("returns true when the password is correct", func(t *testing.T) {
		password := "mysecret1234BxYqMmd7Nhwvw"

		encryptedPassword, err := passwordEncrypter.Encrypt(ctx, password)
		require.NoError(t, err)

		isEqual, err := passwordEncrypter.ComparePassword(ctx, encryptedPassword, password)
		require.NoError(t, err)

		assert.True(t, isEqual)
	})
}
