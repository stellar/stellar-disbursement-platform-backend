package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var errUnexpectedError = errors.New("unexpected error")

func assertUserIsActive(t *testing.T, ctx context.Context, dbConnectionPool db.DBConnectionPool, userID string, expectedIsActive bool) {
	const query = "SELECT is_active FROM auth_users WHERE id = $1"

	var isActive bool
	err := dbConnectionPool.GetContext(ctx, &isActive, query, userID)
	require.NoError(t, err)

	assert.Equal(t, expectedIsActive, isActive)
}

func Test_DefaultAuthenticator_ValidateCredential(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool), withPasswordEncrypter(passwordEncrypterMock))

	ctx := context.Background()

	t.Run("returns error when email is not found", func(t *testing.T) {
		email, pass := "email@email.com", "pass1234"

		user, err := authenticator.ValidateCredentials(ctx, email, pass)

		assert.EqualError(t, err, ErrInvalidCredentials.Error())
		assert.Nil(t, user)
	})

	t.Run("returns error when Password Encrypter fails comparing password and hash", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)

		password := "wrongpassword"
		passwordEncrypterMock.
			On("ComparePassword", ctx, randUser.EncryptedPassword, password).
			Return(false, errUnexpectedError).
			Once()

		user, err := authenticator.ValidateCredentials(ctx, randUser.Email, password)

		assert.EqualError(t, err, "comparing password: unexpected error")
		assert.Nil(t, user)
	})

	t.Run("returns error when password is wrong", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)

		password := "wrongpassword"
		passwordEncrypterMock.
			On("ComparePassword", ctx, randUser.EncryptedPassword, password).
			Return(false, nil).
			Once()

		user, err := authenticator.ValidateCredentials(ctx, randUser.Email, password)

		assert.EqualError(t, err, ErrInvalidCredentials.Error())
		assert.Nil(t, user)
	})

	t.Run("returns error when user is not active", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		err := authenticator.updateIsActive(ctx, randUser.ID, false)
		require.NoError(t, err)

		user, err := authenticator.ValidateCredentials(ctx, randUser.Email, randUser.Password)

		assert.EqualError(t, err, ErrInvalidCredentials.Error())
		assert.Nil(t, user)
	})

	t.Run("returns user successfully", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)

		passwordEncrypterMock.
			On("ComparePassword", ctx, randUser.EncryptedPassword, randUser.Password).
			Return(true, nil).
			Once()

		user, err := authenticator.ValidateCredentials(ctx, randUser.Email, randUser.Password)
		require.NoError(t, err)

		assert.Equal(t, randUser.Email, user.Email)
		assert.Equal(t, randUser.ID, user.ID)
		assert.Equal(t, randUser.FirstName, user.FirstName)
		assert.Equal(t, randUser.LastName, user.LastName)
	})

	passwordEncrypterMock.AssertExpectations(t)
}

func Test_DefaultAuthenticator_CreateUser(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool), withPasswordEncrypter(passwordEncrypterMock))

	ctx := context.Background()

	t.Run("returns error when user is not valid", func(t *testing.T) {
		user := &User{
			Email:     "",
			FirstName: "",
			LastName:  "",
		}

		password := "mysecret"

		// Email
		u, err := authenticator.CreateUser(ctx, user, password)

		assert.Nil(t, u)
		assert.EqualError(t, err, "error validating user fields: email is required")

		user.Email = "invalid"
		u, err = authenticator.CreateUser(ctx, user, password)

		assert.Nil(t, u)
		assert.EqualError(t, err, `error validating user fields: email is invalid: the provided email "invalid" is not valid`)

		// First name
		user.Email = "email@email.com"
		u, err = authenticator.CreateUser(ctx, user, password)

		assert.Nil(t, u)
		assert.EqualError(t, err, "error validating user fields: first name is required")

		// Last name
		user.FirstName = "First"
		u, err = authenticator.CreateUser(ctx, user, password)

		assert.Nil(t, u)
		assert.EqualError(t, err, "error validating user fields: last name is required")
	})

	t.Run("returns error when password is invalid", func(t *testing.T) {
		user := &User{
			Email:     "email@email.com",
			FirstName: "First",
			LastName:  "Last",
		}

		password := "secret"

		passwordEncrypterMock.
			On("Encrypt", ctx, password).
			Return("", ErrPasswordTooShort).
			Once()

		u, err := authenticator.CreateUser(ctx, user, password)

		assert.Nil(t, u)
		assert.EqualError(t, err, fmt.Sprintf("error encrypting password: password should have at least %d characters", MinPasswordLength))

		passwordEncrypterMock.
			On("Encrypt", ctx, password).
			Return("", errUnexpectedError).
			Once()

		u, err = authenticator.CreateUser(ctx, user, password)

		assert.Nil(t, u)
		assert.EqualError(t, err, "error encrypting password: unexpected error")
	})

	t.Run("returns error when user is duplicated", func(t *testing.T) {
		user := &User{
			Email:     "email@email.com",
			FirstName: "First",
			LastName:  "Last",
		}

		password := "mysecret"

		passwordEncrypterMock.
			On("Encrypt", ctx, password).
			Return("encrypted", nil).
			Twice()

		_, err := authenticator.CreateUser(ctx, user, password)
		require.NoError(t, err)

		u, err := authenticator.CreateUser(ctx, user, password)

		assert.Nil(t, u)
		assert.EqualError(t, err, ErrUserEmailAlreadyExists.Error())
	})

	t.Run("creates a new user correctly", func(t *testing.T) {
		user := &User{
			Email:     "email-test@email.com",
			FirstName: "First",
			LastName:  "Last",
		}

		password := "mysecret"

		passwordEncrypterMock.
			On("Encrypt", ctx, password).
			Return("encryptedpassword", nil).
			Once()

		u, err := authenticator.CreateUser(ctx, user, password)
		require.NoError(t, err)

		const query = "SELECT id, email, first_name, last_name, encrypted_password, is_active FROM auth_users WHERE email = $1"

		var newUser User
		var encryptedPassword string
		err = dbConnectionPool.QueryRowxContext(ctx, query, user.Email).Scan(&newUser.ID, &newUser.Email, &newUser.FirstName, &newUser.LastName, &encryptedPassword, &newUser.IsActive)
		require.NoError(t, err)

		assert.Equal(t, newUser.ID, u.ID)
		assert.Equal(t, newUser.Email, u.Email)
		assert.Equal(t, newUser.FirstName, u.FirstName)
		assert.Equal(t, newUser.LastName, u.LastName)
		assert.Equal(t, newUser.IsActive, u.IsActive)
		assert.Equal(t, "encryptedpassword", encryptedPassword)
	})

	t.Run("creates a user successfully with an OTP", func(t *testing.T) {
		user := &User{
			Email:     "emailotp@email.com",
			FirstName: "First",
			LastName:  "Last",
		}

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encryptedpassword", nil).
			Once()

		u, err := authenticator.CreateUser(ctx, user, "")
		require.NoError(t, err)

		const query = "SELECT id, email, first_name, last_name, encrypted_password FROM auth_users WHERE email = $1"

		var newUser User
		var encryptedPassword string
		err = dbConnectionPool.QueryRowxContext(ctx, query, user.Email).Scan(&newUser.ID, &newUser.Email, &newUser.FirstName, &newUser.LastName, &encryptedPassword)
		require.NoError(t, err)

		assert.Equal(t, newUser.ID, u.ID)
		assert.Equal(t, newUser.Email, u.Email)
		assert.Equal(t, newUser.FirstName, u.FirstName)
		assert.Equal(t, newUser.LastName, u.LastName)
		assert.Equal(t, "encryptedpassword", encryptedPassword)
	})

	passwordEncrypterMock.AssertExpectations(t)
}

func Test_DefaultAuthenticator_ActivateUser(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool))

	ctx := context.Background()

	t.Run("returns error when user does not exist", func(t *testing.T) {
		err = authenticator.ActivateUser(ctx, "user-id")
		assert.EqualError(t, err, "error activating user ID user-id: no rows affected")
	})

	t.Run("activate user correctly", func(t *testing.T) {
		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

		err := authenticator.updateIsActive(ctx, randUser.ID, false)
		require.NoError(t, err)
		assertUserIsActive(t, ctx, dbConnectionPool, randUser.ID, false)

		err = authenticator.ActivateUser(ctx, randUser.ID)
		require.NoError(t, err)
		assertUserIsActive(t, ctx, dbConnectionPool, randUser.ID, true)
	})
}

func Test_DefaultAuthenticator_DeactivateUser(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool))

	ctx := context.Background()

	t.Run("returns error when user does not exist", func(t *testing.T) {
		err = authenticator.DeactivateUser(ctx, "user-id")
		assert.EqualError(t, err, "error deactivating user ID user-id: no rows affected")
	})

	t.Run("deactivate user correctly", func(t *testing.T) {
		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, NewDefaultPasswordEncrypter(), false)

		assertUserIsActive(t, ctx, dbConnectionPool, randUser.ID, true)

		err = authenticator.DeactivateUser(ctx, randUser.ID)
		require.NoError(t, err)
		assertUserIsActive(t, ctx, dbConnectionPool, randUser.ID, false)
	})
}

func Test_DefaultAuthenticator_invalidateResetPasswordToken(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool), withPasswordEncrypter(passwordEncrypterMock))

	ctx := context.Background()

	t.Run("Should change status of the token to invalid", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		token := CreateResetPasswordTokenFixture(t, ctx, dbConnectionPool, randUser, true, time.Now())

		dbTx, err := dbConnectionPool.BeginTxx(ctx, nil)
		require.NoError(t, err)

		err = authenticator.invalidateResetPasswordToken(ctx, dbTx, token)
		require.NoError(t, err)

		err = dbTx.Commit()
		require.NoError(t, err)

		var dbToken string
		q := "SELECT token FROM auth_user_password_reset WHERE token = $1 AND is_valid = true"
		err = dbConnectionPool.GetContext(ctx, &dbToken, q, token)
		require.EqualError(t, err, sql.ErrNoRows.Error())
		require.Empty(t, dbToken)
	})

	passwordEncrypterMock.AssertExpectations(t)
}

func Test_DefaultAuthenticator_ResetPassword(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool), withPasswordEncrypter(passwordEncrypterMock))

	ctx := context.Background()

	t.Run("Should treat encrypt password error", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"
		newPassword := "new_not_encrypted_pass"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once().
			On("Encrypt", ctx, newPassword).
			Return("", errUnexpectedError).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		token := CreateResetPasswordTokenFixture(t, ctx, dbConnectionPool, randUser, true, time.Now())

		err := authenticator.ResetPassword(ctx, token, newPassword)
		assert.EqualError(t, err, "running atomic function in RunInTransactionWithResult: error trying to encrypt user password: unexpected error")
	})

	t.Run("Should treat a not found token error", func(t *testing.T) {
		err := authenticator.ResetPassword(ctx, "notfoundtoken", "newpassword")
		assert.EqualError(t, err, "running atomic function in RunInTransactionWithResult: "+ErrInvalidResetPasswordToken.Error())
	})

	t.Run("Should reset the password with a valid token, and make the token invalid after", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"
		newPassword := "new_not_encrypted_pass"
		newEncryptedPassword := "newencryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once().
			On("Encrypt", ctx, newPassword).
			Return(newEncryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		token := CreateResetPasswordTokenFixture(t, ctx, dbConnectionPool, randUser, true, time.Now())

		err := authenticator.ResetPassword(ctx, token, newPassword)
		require.NoError(t, err)

		// Token should be invalid after
		var dbIsValid bool
		q := `SELECT is_valid FROM auth_user_password_reset WHERE token = $1`
		err = dbConnectionPool.GetContext(ctx, &dbIsValid, q, token)
		require.NoError(t, err)
		assert.False(t, dbIsValid)

		// User should have a new password encrypted
		var expectedNewEncryptedPass string
		q = `SELECT encrypted_password FROM auth_users WHERE id = $1`
		err = dbConnectionPool.GetContext(ctx, &expectedNewEncryptedPass, q, randUser.ID)
		require.NoError(t, err)
		assert.Equal(t, expectedNewEncryptedPass, newEncryptedPassword)
	})

	t.Run("Should return an error with an expired token", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"
		newPassword := "new_not_encrypted_pass"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		token := CreateResetPasswordTokenFixture(t, ctx, dbConnectionPool, randUser, true, time.Now().Add(-time.Hour*25))

		err := authenticator.ResetPassword(ctx, token, newPassword)
		require.EqualError(t, err, "running atomic function in RunInTransactionWithResult: "+ErrInvalidResetPasswordToken.Error())
	})

	passwordEncrypterMock.AssertExpectations(t)
}

func Test_DefaultAuthenticator_ForgotPassword(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool), withPasswordEncrypter(passwordEncrypterMock))

	ctx := context.Background()

	t.Run("Should return an error if the email is empty", func(t *testing.T) {
		resetToken, err := authenticator.ForgotPassword(ctx, "")
		assert.EqualError(t, err, "error generating user reset password token: email cannot be empty")
		assert.Empty(t, resetToken)
	})

	t.Run("Should return an error if the user is not found", func(t *testing.T) {
		resetToken, err := authenticator.ForgotPassword(ctx, "notfounduser@email.com")
		assert.EqualError(t, err, ErrUserNotFound.Error())
		assert.Empty(t, resetToken)
	})

	t.Run("should return an error if user has valid token", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)

		resetToken, err := authenticator.ForgotPassword(ctx, randUser.Email)
		require.NoError(t, err)
		assert.NotEmpty(t, resetToken)

		resetTokenFail1, err := authenticator.ForgotPassword(ctx, randUser.Email)
		require.EqualError(t, err, "user has a valid token")
		assert.Empty(t, resetTokenFail1)

		updateTokenQuery := `
			UPDATE auth_user_password_reset
			SET created_at = (created_at - INTERVAL '19 minutes')
			WHERE token = $1
		`
		_, err = dbConnectionPool.ExecContext(ctx, updateTokenQuery, resetToken)
		require.NoError(t, err)

		resetTokenFail2, err := authenticator.ForgotPassword(ctx, randUser.Email)
		require.EqualError(t, err, "user has a valid token")
		assert.Empty(t, resetTokenFail2)
	})

	t.Run("should return reset token when previous token is expired", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)

		oldResetToken, err := authenticator.ForgotPassword(ctx, randUser.Email)
		require.NoError(t, err)
		assert.NotEmpty(t, oldResetToken)

		// Expire old token
		updateTokenQuery := `
			UPDATE auth_user_password_reset
			SET created_at = (created_at - INTERVAL '20 minutes')
			WHERE token = $1
		`
		_, err = dbConnectionPool.ExecContext(ctx, updateTokenQuery, oldResetToken)
		require.NoError(t, err)

		newResetToken, err := authenticator.ForgotPassword(ctx, randUser.Email)
		require.NoError(t, err)
		assert.NotEmpty(t, newResetToken)
		assert.NotEqual(t, oldResetToken, newResetToken)
	})

	t.Run("Should return reset token with a valid user", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)

		resetToken, err := authenticator.ForgotPassword(ctx, randUser.Email)
		require.NoError(t, err)

		assert.NotEmpty(t, resetToken)
	})

	passwordEncrypterMock.AssertExpectations(t)
}

func Test_withResetTokenExpirationHours(t *testing.T) {
	authenticator := newDefaultAuthenticator(withResetTokenExpirationHours(time.Hour * 24))
	assert.Equal(t, time.Hour*24, authenticator.resetTokenExpirationHours)

	authenticator = newDefaultAuthenticator(withResetTokenExpirationHours(time.Minute * 30))
	assert.Equal(t, time.Minute*30, authenticator.resetTokenExpirationHours)
}

func Test_DefaultAuthenticator_GetAllUsers(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool))

	ctx := context.Background()

	t.Run("returns an empty array if no users are registered", func(t *testing.T) {
		users, err := authenticator.GetAllUsers(ctx)
		require.NoError(t, err)

		assert.Empty(t, users)
	})

	t.Run("gets all users successfully", func(t *testing.T) {
		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encryptedPassword", nil)

		randUser1 := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false, "role1", "role2")
		randUser2 := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, true, "role1", "role2")
		randUser3 := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false, "role3")

		users, err := authenticator.GetAllUsers(ctx)
		require.NoError(t, err)

		expectedUsers := []User{
			*randUser1.ToUser(),
			*randUser2.ToUser(),
			*randUser3.ToUser(),
		}

		assert.Equal(t, expectedUsers, users)
	})

	passwordEncrypterMock.AssertExpectations(t)
}

func Test_DefaultAuthenticator_GetUsers(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool))

	ctx := context.Background()

	t.Run("returns an error if users for user IDs cannot be found", func(t *testing.T) {
		userIDs := []string{"invalid-id"}
		_, err := authenticator.GetUsers(ctx, userIDs)
		require.EqualError(t, err, "error querying user IDs: searching for 1 users, found 0 users")
	})

	t.Run("gets users for provided IDs successfully", func(t *testing.T) {
		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encryptedPassword", nil)

		randUser1 := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false, "role1", "role2")
		randUser2 := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, true, "role1", "role2")
		randUser3 := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false, "role3")

		users, err := authenticator.GetUsers(
			ctx, []string{randUser1.ID, randUser2.ID, randUser3.ID},
		)
		require.NoError(t, err)

		expectedUsers := []*User{
			{
				ID:        randUser1.ID,
				FirstName: randUser1.FirstName,
				LastName:  randUser1.LastName,
			},
			{
				ID:        randUser2.ID,
				FirstName: randUser2.FirstName,
				LastName:  randUser2.LastName,
			},
			{
				ID:        randUser3.ID,
				FirstName: randUser3.FirstName,
				LastName:  randUser3.LastName,
			},
		}

		assert.Equal(t, expectedUsers, users)
	})

	passwordEncrypterMock.AssertExpectations(t)
}

func Test_DefaultAuthenticator_UpdateUser(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(
		withAuthenticatorDatabaseConnectionPool(dbConnectionPool),
		withPasswordEncrypter(passwordEncrypterMock),
	)

	ctx := context.Background()

	type dbUser struct {
		ID                string `db:"id"`
		FirstName         string `db:"first_name"`
		LastName          string `db:"last_name"`
		Email             string `db:"email"`
		EncryptedPassword string `db:"encrypted_password"`
	}

	getUser := func(t *testing.T, ctx context.Context, ID string) *dbUser {
		const query = `
			SELECT id, first_name, last_name, email, encrypted_password FROM auth_users WHERE id = $1
		`
		var u dbUser
		err := dbConnectionPool.GetContext(ctx, &u, query, ID)
		require.NoError(t, err)

		return &u
	}

	t.Run("returns error when no value is provided", func(t *testing.T) {
		err := authenticator.UpdateUser(ctx, "user-id", "", "", "", "")
		assert.EqualError(t, err, "provide at least one of these values: firstName, lastName, email or password")
	})

	t.Run("returns error when email is invalid", func(t *testing.T) {
		err := authenticator.UpdateUser(ctx, "user-id", "", "", "invalid", "")
		assert.EqualError(t, err, `error validating email: the provided email "invalid" is not valid`)
	})

	t.Run("returns error when password is too short", func(t *testing.T) {
		password := "short"

		passwordEncrypterMock.
			On("Encrypt", ctx, password).
			Return("", ErrPasswordTooShort).
			Once()

		err := authenticator.UpdateUser(ctx, "user-id", "", "", "", "short")
		assert.EqualError(t, err, fmt.Sprintf("password should have at least %d characters", MinPasswordLength))
	})

	t.Run("returns error when PasswordEncrypter fails", func(t *testing.T) {
		password := "short"

		passwordEncrypterMock.
			On("Encrypt", ctx, password).
			Return("", errUnexpectedError).
			Once()

		err := authenticator.UpdateUser(ctx, "user-id", "", "", "", "short")
		assert.EqualError(t, err, "error encrypting password: unexpected error")
	})

	t.Run("updates first name successfully", func(t *testing.T) {
		firstName := "FirstName"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encrypted", nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		assert.NotEqual(t, firstName, randUser.FirstName)

		err := authenticator.UpdateUser(ctx, randUser.ID, firstName, "", "", "")
		require.NoError(t, err)

		u := getUser(t, ctx, randUser.ID)

		assert.Equal(t, firstName, u.FirstName)
		assert.Equal(t, randUser.LastName, u.LastName)
		assert.Equal(t, randUser.Email, u.Email)
		assert.Equal(t, randUser.EncryptedPassword, u.EncryptedPassword)
	})

	t.Run("updates last name successfully", func(t *testing.T) {
		lastName := "LastName"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encrypted", nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		assert.NotEqual(t, lastName, randUser.LastName)

		err := authenticator.UpdateUser(ctx, randUser.ID, "", lastName, "", "")
		require.NoError(t, err)

		u := getUser(t, ctx, randUser.ID)

		assert.Equal(t, lastName, u.LastName)
		assert.Equal(t, randUser.FirstName, u.FirstName)
		assert.Equal(t, randUser.Email, u.Email)
		assert.Equal(t, randUser.EncryptedPassword, u.EncryptedPassword)
	})

	t.Run("updates email successfully", func(t *testing.T) {
		email := "email@email.com"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encrypted", nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		assert.NotEqual(t, email, randUser.Email)

		err := authenticator.UpdateUser(ctx, randUser.ID, "", "", email, "")
		require.NoError(t, err)

		u := getUser(t, ctx, randUser.ID)

		assert.Equal(t, email, u.Email)
		assert.Equal(t, randUser.FirstName, u.FirstName)
		assert.Equal(t, randUser.LastName, u.LastName)
		assert.Equal(t, randUser.EncryptedPassword, u.EncryptedPassword)
	})

	t.Run("updates password successfully", func(t *testing.T) {
		password := "newpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encrypted", nil).
			Once().
			On("Encrypt", ctx, password).
			Return("newpassowrdencrypted", nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		assert.NotEqual(t, "newpassowrdencrypted", randUser.EncryptedPassword)

		err := authenticator.UpdateUser(ctx, randUser.ID, "", "", "", password)
		require.NoError(t, err)

		u := getUser(t, ctx, randUser.ID)

		assert.Equal(t, "newpassowrdencrypted", u.EncryptedPassword)
		assert.Equal(t, randUser.FirstName, u.FirstName)
		assert.Equal(t, randUser.LastName, u.LastName)
		assert.Equal(t, randUser.Email, u.Email)
	})

	t.Run("updates all fields successfully", func(t *testing.T) {
		firstName, lastName, email, password := "FirstName", "LastName", "new_email@email.com", "newpassword"

		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encrypted", nil).
			Once().
			On("Encrypt", ctx, password).
			Return("newpassowrdencrypted", nil).
			Once()

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		assert.NotEqual(t, firstName, randUser.FirstName)
		assert.NotEqual(t, lastName, randUser.LastName)
		assert.NotEqual(t, email, randUser.Email)
		assert.NotEqual(t, "newpassowrdencrypted", randUser.EncryptedPassword)

		err := authenticator.UpdateUser(ctx, randUser.ID, firstName, lastName, email, password)
		require.NoError(t, err)

		u := getUser(t, ctx, randUser.ID)

		assert.Equal(t, firstName, u.FirstName)
		assert.Equal(t, lastName, u.LastName)
		assert.Equal(t, email, u.Email)
		assert.Equal(t, "newpassowrdencrypted", u.EncryptedPassword)
	})
}

func Test_DefaultAuthenticator_GetUser(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool))

	ctx := context.Background()

	t.Run("returns error when user is not found", func(t *testing.T) {
		user, err := authenticator.GetUser(ctx, "user-id")
		assert.ErrorIs(t, err, ErrUserNotFound)
		assert.Nil(t, user)
	})

	t.Run("returns user successfully", func(t *testing.T) {
		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return("encryptedPassword", nil)

		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false, "role1")

		u, err := authenticator.GetUser(ctx, randUser.ID)
		require.NoError(t, err)

		assert.Equal(t, randUser.ID, u.ID)
		assert.Equal(t, randUser.FirstName, u.FirstName)
		assert.Equal(t, randUser.LastName, u.LastName)
		assert.Equal(t, randUser.Email, u.Email)
	})
}

func Test_DefaultAuthenticator_UpdatePassword(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()
	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	passwordEncrypterMock := &PasswordEncrypterMock{}
	authenticator := newDefaultAuthenticator(withAuthenticatorDatabaseConnectionPool(dbConnectionPool), withPasswordEncrypter(passwordEncrypterMock))

	ctx := context.Background()

	type dbUser struct {
		ID                string `db:"id"`
		FirstName         string `db:"first_name"`
		LastName          string `db:"last_name"`
		Email             string `db:"email"`
		EncryptedPassword string `db:"encrypted_password"`
	}

	getUser := func(t *testing.T, ctx context.Context, ID string) *dbUser {
		const query = `
			SELECT id, first_name, last_name, email, encrypted_password FROM auth_users WHERE id = $1
		`
		var u dbUser
		err := dbConnectionPool.GetContext(ctx, &u, query, ID)
		require.NoError(t, err)

		return &u
	}

	t.Run("returns error when no value is provided", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"
		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()
		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		err := authenticator.UpdatePassword(ctx, randUser.ToUser(), "", "")
		assert.EqualError(t, err, "provide currentPassword and newPassword values")
	})

	t.Run("returns error when credentials are invalid", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"
		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once()
		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		passwordEncrypterMock.
			On("ComparePassword", ctx, randUser.EncryptedPassword, randUser.Password).
			Return(false, nil).
			Once()
		err := authenticator.UpdatePassword(ctx, randUser.ToUser(), randUser.Password, "newpassword")
		assert.EqualError(t, err, "validating credentials: invalid credentials")
	})

	t.Run("returns error when encrypting new password fails", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"
		newPassword := "new_not_encrypted_pass"
		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once().
			On("Encrypt", ctx, newPassword).
			Return("", errUnexpectedError).
			Once()
		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		passwordEncrypterMock.
			On("ComparePassword", ctx, randUser.EncryptedPassword, randUser.Password).
			Return(true, nil).
			Once()

		err := authenticator.UpdatePassword(ctx, randUser.ToUser(), randUser.Password, newPassword)
		assert.EqualError(t, err, "encrypting password: unexpected error")
	})

	t.Run("updates password successfully", func(t *testing.T) {
		encryptedPassword := "encryptedpassword"
		newPassword := "new_not_encrypted_pass"
		newEncryptedPassword := "newencryptedpassword"
		passwordEncrypterMock.
			On("Encrypt", ctx, mock.AnythingOfType("string")).
			Return(encryptedPassword, nil).
			Once().
			On("Encrypt", ctx, newPassword).
			Return(newEncryptedPassword, nil).
			Once()
		randUser := CreateRandomAuthUserFixture(t, ctx, dbConnectionPool, passwordEncrypterMock, false)
		assert.NotEqual(t, newEncryptedPassword, randUser.EncryptedPassword)

		passwordEncrypterMock.
			On("ComparePassword", ctx, randUser.EncryptedPassword, randUser.Password).
			Return(true, nil).
			Once()

		err := authenticator.UpdatePassword(ctx, randUser.ToUser(), randUser.Password, newPassword)
		require.NoError(t, err)

		u := getUser(t, ctx, randUser.ID)
		assert.Equal(t, newEncryptedPassword, u.EncryptedPassword)
	})
}
