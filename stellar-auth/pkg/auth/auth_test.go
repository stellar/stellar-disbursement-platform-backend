package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_AuthManager_Authenticate(t *testing.T) {
	authenticatorMock := &AuthenticatorMock{}
	jwtManagerMock := &JWTManagerMock{}
	roleManagerMock := &RoleManagerMock{}

	authManager := NewAuthManager(
		WithCustomAuthenticatorOption(authenticatorMock),
		WithCustomJWTManagerOption(jwtManagerMock),
		WithCustomRoleManagerOption(roleManagerMock),
	)

	ctx := context.Background()

	t.Run("returns error when invalid credentials is provided", func(t *testing.T) {
		email, password := "email@email.com", "pass123"

		authenticatorMock.
			On("ValidateCredentials", ctx, email, password).
			Return(nil, errors.New("invalid credentials")).
			Once()

		token, err := authManager.Authenticate(ctx, email, password)

		assert.EqualError(t, err, "validating credentials: invalid credentials")
		assert.Empty(t, token)
	})

	t.Run("returns error when get user roles fails", func(t *testing.T) {
		email, password := "email@email.com", "pass123"

		expectedUser := &User{
			ID:    "user-id",
			Email: "email@email.com",
		}

		authenticatorMock.
			On("ValidateCredentials", ctx, email, password).
			Return(expectedUser, nil).
			Once()

		roleManagerMock.
			On("GetUserRoles", ctx, expectedUser).
			Return(nil, errUnexpectedError).
			Once()

		token, err := authManager.Authenticate(ctx, email, password)

		assert.EqualError(t, err, "error getting user roles: unexpected error")
		assert.Empty(t, token)
	})

	t.Run("returns error when generate token fails", func(t *testing.T) {
		email, password := "email@email.com", "pass123"

		expectedUser := &User{
			ID:    "user-id",
			Email: "email@email.com",
		}

		authenticatorMock.
			On("ValidateCredentials", ctx, email, password).
			Return(expectedUser, nil).
			Once()

		roleManagerMock.
			On("GetUserRoles", ctx, expectedUser).
			Return([]string{"role1"}, nil).
			Once()

		jwtManagerMock.
			On("GenerateToken", ctx, expectedUser, mock.AnythingOfType("time.Time")).
			Return("", errUnexpectedError).
			Once()

		token, err := authManager.Authenticate(ctx, email, password)

		assert.EqualError(t, err, "generating token: unexpected error")
		assert.Empty(t, token)
	})

	t.Run("returns the user JWT token successfully", func(t *testing.T) {
		email, password := "email@email.com", "pass123"

		user := &User{
			ID:    "user-id",
			Email: "email@email.com",
		}

		roles := []string{"role1"}

		expectedUser := &User{
			ID:    "user-id",
			Email: "email@email.com",
			Roles: roles,
		}

		authenticatorMock.
			On("ValidateCredentials", ctx, email, password).
			Return(user, nil).
			Once()

		roleManagerMock.
			On("GetUserRoles", ctx, user).
			Return(roles, nil).
			Once()

		expectedToken := "mytoken"
		jwtManagerMock.
			On("GenerateToken", ctx, expectedUser, mock.AnythingOfType("time.Time")).
			Return(expectedToken, nil).
			Once()

		token, err := authManager.Authenticate(ctx, email, password)
		require.NoError(t, err)

		assert.Equal(t, expectedToken, token)
	})

	authenticatorMock.AssertExpectations(t)
	jwtManagerMock.AssertExpectations(t)
	roleManagerMock.AssertExpectations(t)
}

func Test_AuthManager_ValidateToken(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		isValid, err := authManager.ValidateToken(ctx, token)

		assert.EqualError(t, err, "validating token: unexpected error")
		assert.False(t, isValid)
	})

	t.Run("returns false when token is invalid", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		isValid, err := authManager.ValidateToken(ctx, token)
		require.NoError(t, err)

		assert.False(t, isValid)
	})

	t.Run("returns true when token is valid", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil)

		isValid, err := authManager.ValidateToken(ctx, token)
		require.NoError(t, err)

		assert.True(t, isValid)
	})

	jwtManagerMock.AssertExpectations(t)
}

func Test_AuthManager_RefreshToken(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		refreshedToken, err := authManager.RefreshToken(ctx, token)

		assert.EqualError(t, err, "validating token: validating token: unexpected error")
		assert.Empty(t, refreshedToken)
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		refreshedToken, err := authManager.RefreshToken(ctx, token)

		assert.EqualError(t, err, ErrInvalidToken.Error())
		assert.Empty(t, refreshedToken)
	})

	t.Run("returns error when JWT Manager fails", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("RefreshToken", ctx, token, mock.AnythingOfType("time.Time")).
			Return("", errUnexpectedError).
			Once()

		refreshedToken, err := authManager.RefreshToken(ctx, token)

		assert.EqualError(t, err, "generating new refreshed token: unexpected error")
		assert.Empty(t, refreshedToken)
	})

	t.Run("returns a new token successfully", func(t *testing.T) {
		token := "myoldtoken"
		newToken := "myfreshtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("RefreshToken", ctx, token, mock.AnythingOfType("time.Time")).
			Return(newToken, nil).
			Once()

		refreshedToken, err := authManager.RefreshToken(ctx, token)
		require.NoError(t, err)

		assert.NotEqual(t, token, refreshedToken)
		assert.Equal(t, newToken, refreshedToken)
	})

	jwtManagerMock.AssertExpectations(t)
}

func Test_AuthManager_AllRolesInTokenUser(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	roleManagerMock := &RoleManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock), WithCustomRoleManagerOption(roleManagerMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		isValid, err := authManager.AllRolesInTokenUser(ctx, token, []string{"role1"})

		assert.EqualError(t, err, "validating token: validating token: unexpected error")
		assert.False(t, isValid)
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		isValid, err := authManager.AllRolesInTokenUser(ctx, token, []string{"role1"})

		assert.EqualError(t, err, ErrInvalidToken.Error())
		assert.False(t, isValid)
	})

	t.Run("returns error when JWT Manager fails getting user from token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(nil, errUnexpectedError).
			Once()

		isValid, err := authManager.AllRolesInTokenUser(ctx, token, []string{"role1"})

		assert.EqualError(t, err, "error getting user from token: unexpected error")
		assert.False(t, isValid)
	})

	t.Run("returns error when Role Manager fails verifying if user has roles", func(t *testing.T) {
		token := "myoldtoken"

		user := &User{
			ID:    "user-ID",
			Email: "email@email.com",
			Roles: []string{"role1"},
		}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(user, nil).
			Once()

		roleManagerMock.
			On("HasAllRoles", ctx, user, []string{"role1"}).
			Return(false, errUnexpectedError).
			Once()

		isValid, err := authManager.AllRolesInTokenUser(ctx, token, []string{"role1"})

		assert.EqualError(t, err, "error validating user roles: unexpected error")
		assert.False(t, isValid)
	})

	t.Run("validates the user roles correctly", func(t *testing.T) {
		token := "myoldtoken"

		user := &User{
			ID:    "user-ID",
			Email: "email@email.com",
			Roles: []string{"role1", "role3"},
		}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Times(4).
			On("GetUserFromToken", ctx, token).
			Return(user, nil).
			Times(4)

		roleManagerMock.
			On("HasAllRoles", ctx, user, []string{"role1"}).
			Return(true, nil).
			Once()

		isValid, err := authManager.AllRolesInTokenUser(ctx, token, []string{"role1"})
		require.NoError(t, err)
		assert.True(t, isValid)

		roleManagerMock.
			On("HasAllRoles", ctx, user, []string{"role2", "role3"}).
			Return(false, nil).
			Once()

		isValid, err = authManager.AllRolesInTokenUser(ctx, token, []string{"role2", "role3"})
		require.NoError(t, err)
		assert.False(t, isValid)

		roleManagerMock.
			On("HasAllRoles", ctx, user, []string{"role2"}).
			Return(false, nil).
			Once()

		isValid, err = authManager.AllRolesInTokenUser(ctx, token, []string{"role2"})
		require.NoError(t, err)
		assert.False(t, isValid)

		roleManagerMock.
			On("HasAllRoles", ctx, user, []string{"role1", "role3"}).
			Return(true, nil).
			Once()

		isValid, err = authManager.AllRolesInTokenUser(ctx, token, []string{"role1", "role3"})
		require.NoError(t, err)
		assert.True(t, isValid)
	})

	jwtManagerMock.AssertExpectations(t)
	roleManagerMock.AssertExpectations(t)
}

func Test_AuthManager_AnyRolesInTokenUser(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	roleManagerMock := &RoleManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock), WithCustomRoleManagerOption(roleManagerMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		isValid, err := authManager.AnyRolesInTokenUser(ctx, token, []string{"role1"})

		assert.EqualError(t, err, "validating token: validating token: unexpected error")
		assert.False(t, isValid)
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		isValid, err := authManager.AnyRolesInTokenUser(ctx, token, []string{"role1"})

		assert.EqualError(t, err, ErrInvalidToken.Error())
		assert.False(t, isValid)
	})

	t.Run("returns error when JWT Manager fails getting user from token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(nil, errUnexpectedError).
			Once()

		isValid, err := authManager.AnyRolesInTokenUser(ctx, token, []string{"role1"})

		assert.EqualError(t, err, "error getting user from token: unexpected error")
		assert.False(t, isValid)
	})

	t.Run("returns error when Role Manager fails verifying if user has roles", func(t *testing.T) {
		token := "myoldtoken"

		user := &User{
			ID:    "user-ID",
			Email: "email@email.com",
			Roles: []string{"role1"},
		}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(user, nil).
			Once()

		roleManagerMock.
			On("HasAnyRoles", ctx, user, []string{"role1"}).
			Return(false, errUnexpectedError).
			Once()

		isValid, err := authManager.AnyRolesInTokenUser(ctx, token, []string{"role1"})

		assert.EqualError(t, err, "error validating user roles: unexpected error")
		assert.False(t, isValid)
	})

	t.Run("validates the user roles correctly", func(t *testing.T) {
		token := "myoldtoken"

		user := &User{
			ID:    "user-ID",
			Email: "email@email.com",
			Roles: []string{"role1", "role3"},
		}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Times(4).
			On("GetUserFromToken", ctx, token).
			Return(user, nil).
			Times(4)

		roleManagerMock.
			On("HasAnyRoles", ctx, user, []string{"role1"}).
			Return(true, nil).
			Once()

		isValid, err := authManager.AnyRolesInTokenUser(ctx, token, []string{"role1"})
		require.NoError(t, err)
		assert.True(t, isValid)

		roleManagerMock.
			On("HasAnyRoles", ctx, user, []string{"role2", "role3"}).
			Return(true, nil).
			Once()

		isValid, err = authManager.AnyRolesInTokenUser(ctx, token, []string{"role2", "role3"})
		require.NoError(t, err)
		assert.True(t, isValid)

		roleManagerMock.
			On("HasAnyRoles", ctx, user, []string{"role2"}).
			Return(false, nil).
			Once()

		isValid, err = authManager.AnyRolesInTokenUser(ctx, token, []string{"role2"})
		require.NoError(t, err)
		assert.False(t, isValid)

		roleManagerMock.
			On("HasAnyRoles", ctx, user, []string{"role1", "role3"}).
			Return(true, nil).
			Once()

		isValid, err = authManager.AnyRolesInTokenUser(ctx, token, []string{"role1", "role3"})
		require.NoError(t, err)
		assert.True(t, isValid)
	})

	jwtManagerMock.AssertExpectations(t)
	roleManagerMock.AssertExpectations(t)
}

func Test_AuthManager_CreateUser(t *testing.T) {
	authenticatorMock := &AuthenticatorMock{}

	authManager := NewAuthManager(
		WithCustomAuthenticatorOption(authenticatorMock),
	)

	ctx := context.Background()

	t.Run("returns error when Authenticator fails creating user", func(t *testing.T) {
		user := &User{
			Email:     "email@email.com",
			FirstName: "First",
			LastName:  "Last",
		}

		password := "mysecret"

		authenticatorMock.
			On("CreateUser", ctx, user, password).
			Return(nil, errUnexpectedError).
			Once()

		u, err := authManager.CreateUser(ctx, user, password)

		assert.EqualError(t, err, "error creating user: unexpected error")
		assert.Nil(t, u)
	})

	t.Run("create user correctly", func(t *testing.T) {
		newUser := &User{
			Email:     "email@email.com",
			FirstName: "First",
			LastName:  "Last",
		}

		password := "mysecret"

		expectedUser := &User{
			ID:        "user-id",
			Email:     "email@email.com",
			FirstName: "First",
			LastName:  "Last",
		}

		authenticatorMock.
			On("CreateUser", ctx, newUser, password).
			Return(expectedUser, nil).
			Once()

		u, err := authManager.CreateUser(ctx, newUser, password)
		require.NoError(t, err)

		assert.Equal(t, expectedUser, u)
	})

	authenticatorMock.AssertExpectations(t)
}

func Test_AuthManager_ActivateUser(t *testing.T) {
	authenticatorMock := &AuthenticatorMock{}
	jwtManagerMock := &JWTManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock), WithCustomAuthenticatorOption(authenticatorMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		err := authManager.ActivateUser(ctx, token, "user-id")

		assert.EqualError(t, err, "validating token: validating token: unexpected error")
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		err := authManager.ActivateUser(ctx, token, "user-id")

		assert.EqualError(t, err, "invalid token")
	})

	t.Run("returns error when Authenticator fails", func(t *testing.T) {
		token := "mytoken"
		userID := "user-id"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("ActivateUser", ctx, userID).
			Return(errUnexpectedError).
			Once()

		err := authManager.ActivateUser(ctx, token, userID)

		assert.EqualError(t, err, "error activating user ID user-id: unexpected error")
	})

	t.Run("activate user successfully", func(t *testing.T) {
		token := "mytoken"
		userID := "user-id"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("ActivateUser", ctx, userID).
			Return(nil).
			Once()

		err := authManager.ActivateUser(ctx, token, userID)

		assert.Nil(t, err)
	})
}

func Test_AuthManager_UpdateUser(t *testing.T) {
	authenticatorMock := &AuthenticatorMock{}
	jwtManagerMock := &JWTManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock), WithCustomAuthenticatorOption(authenticatorMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		err := authManager.UpdateUser(ctx, token, "First", "Last", "email@email.com", "mysecret")

		assert.EqualError(t, err, "validating token: validating token: unexpected error")

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(nil, errUnexpectedError).
			Once()

		err = authManager.UpdateUser(ctx, token, "First", "Last", "email@email.com", "mysecret")

		assert.EqualError(t, err, "getting user from token: unexpected error")
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		err := authManager.UpdateUser(ctx, token, "First", "Last", "email@email.com", "mysecret")

		assert.EqualError(t, err, "invalid token")
	})

	t.Run("returns error when Authenticator fails", func(t *testing.T) {
		token := "mytoken"
		userID := "user-id"
		firstName, lastName, email, password := "First", "Last", "email@email.com", "mysecret"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(&User{ID: userID}, nil).
			Once()

		authenticatorMock.
			On("UpdateUser", ctx, userID, firstName, lastName, email, password).
			Return(errUnexpectedError).
			Once()

		err := authManager.UpdateUser(ctx, token, "First", "Last", "email@email.com", "mysecret")

		assert.EqualError(t, err, "error updating user: unexpected error")
	})

	t.Run("updates user successfully", func(t *testing.T) {
		token := "mytoken"
		userID := "user-id"
		firstName, lastName, email, password := "First", "Last", "email@email.com", "mysecret"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(&User{ID: userID}, nil).
			Once()

		authenticatorMock.
			On("UpdateUser", ctx, userID, firstName, lastName, email, password).
			Return(nil).
			Once()

		err := authManager.UpdateUser(ctx, token, "First", "Last", "email@email.com", "mysecret")

		assert.Nil(t, err)
	})
}

func Test_AuthManager_DeactivateUser(t *testing.T) {
	authenticatorMock := &AuthenticatorMock{}
	jwtManagerMock := &JWTManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock), WithCustomAuthenticatorOption(authenticatorMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		err := authManager.DeactivateUser(ctx, token, "user-id")

		assert.EqualError(t, err, "validating token: validating token: unexpected error")
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		err := authManager.DeactivateUser(ctx, token, "user-id")

		assert.EqualError(t, err, "invalid token")
	})

	t.Run("returns error when Authenticator fails", func(t *testing.T) {
		token := "mytoken"
		userID := "user-id"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("DeactivateUser", ctx, userID).
			Return(errUnexpectedError).
			Once()

		err := authManager.DeactivateUser(ctx, token, userID)

		assert.EqualError(t, err, "error deactivating user ID user-id: unexpected error")
	})

	t.Run("deactivate user successfully", func(t *testing.T) {
		token := "mytoken"
		userID := "user-id"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("DeactivateUser", ctx, userID).
			Return(nil).
			Once()

		err := authManager.DeactivateUser(ctx, token, userID)

		assert.Nil(t, err)
	})
}

func Test_AuthManager_UpdateUserRoles(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	roleManagerMock := &RoleManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock), WithCustomRoleManagerOption(roleManagerMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		err := authManager.UpdateUserRoles(ctx, token, "user-id", []string{"role1"})

		assert.EqualError(t, err, "validating token: validating token: unexpected error")
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		err := authManager.UpdateUserRoles(ctx, token, "user-id", []string{"role1"})

		assert.EqualError(t, err, "invalid token")
	})

	t.Run("returns error when Authenticator fails", func(t *testing.T) {
		token := "mytoken"
		userID := "user-id"
		roles := []string{"role1"}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once()

		roleManagerMock.
			On("UpdateRoles", ctx, &User{ID: userID}, roles).
			Return(errUnexpectedError).
			Once()

		err := authManager.UpdateUserRoles(ctx, token, userID, roles)

		assert.EqualError(t, err, "error updating user roles: unexpected error")
	})

	t.Run("update user roles successfully", func(t *testing.T) {
		token := "mytoken"
		userID := "user-id"
		roles := []string{"role1"}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once()

		roleManagerMock.
			On("UpdateRoles", ctx, &User{ID: userID}, roles).
			Return(nil).
			Once()

		err := authManager.UpdateUserRoles(ctx, token, userID, roles)

		assert.Nil(t, err)
	})
}

func Test_AuthManager_WithExpirationTimeInMinutesOption(t *testing.T) {
	authManager := NewAuthManager(WithExpirationTimeInMinutesOption(10))
	assert.Equal(t, time.Minute*10, authManager.ExpirationTimeInMinutes())
}

func Test_AuthManager_ForgotPassword(t *testing.T) {
	authenticatorMock := &AuthenticatorMock{}
	authManager := NewAuthManager(
		WithCustomAuthenticatorOption(authenticatorMock),
	)

	ctx := context.Background()

	t.Run("returns error when user is not found", func(t *testing.T) {
		authenticatorMock.
			On("ForgotPassword", ctx, "wrongemail@email.com").
			Return("", ErrUserNotFound).
			Once()

		resetToken, err := authManager.ForgotPassword(ctx, "wrongemail@email.com")
		assert.EqualError(t, err, "user not found in auth forgot password: user not found")
		assert.Empty(t, resetToken)
	})

	t.Run("returns error when authenticator fails", func(t *testing.T) {
		authenticatorMock.
			On("ForgotPassword", ctx, "wrongemail@email.com").
			Return("", errUnexpectedError).
			Once()

		resetToken, err := authManager.ForgotPassword(ctx, "wrongemail@email.com")
		assert.EqualError(t, err, "error on forgot password: unexpected error")
		assert.Empty(t, resetToken)
	})

	t.Run("creates a reset token successfully", func(t *testing.T) {
		authenticatorMock.
			On("ForgotPassword", ctx, "valid@email.com").
			Return("resettoken", nil).
			Once()

		resetToken, err := authManager.ForgotPassword(ctx, "valid@email.com")
		require.NoError(t, err)
		assert.Equal(t, "resettoken", resetToken)
	})

	authenticatorMock.AssertExpectations(t)
}

func Test_AuthManager_ResetPassword(t *testing.T) {
	authenticatorMock := &AuthenticatorMock{}
	authManager := NewAuthManager(
		WithCustomAuthenticatorOption(authenticatorMock),
	)

	ctx := context.Background()

	t.Run("returns error when the reset token is invalid", func(t *testing.T) {
		authenticatorMock.
			On("ResetPassword", ctx, "invalidToken", "password123").
			Return(ErrInvalidResetPasswordToken).
			Once()

		err := authManager.ResetPassword(ctx, "invalidToken", "password123")
		require.EqualError(t, err, "invalid token in auth reset password: invalid reset password token")
	})

	t.Run("returns error when authenticator fails", func(t *testing.T) {
		authenticatorMock.
			On("ResetPassword", ctx, "validToken", "password123").
			Return(errUnexpectedError).
			Once()

		err := authManager.ResetPassword(ctx, "validToken", "password123")
		assert.EqualError(t, err, "error on reset password: unexpected error")
	})

	t.Run("no error with a valid reset token", func(t *testing.T) {
		authenticatorMock.
			On("ResetPassword", ctx, "goodtoken", "password123").
			Return(nil).
			Once()

		err := authManager.ResetPassword(ctx, "goodtoken", "password123")
		require.NoError(t, err)
	})

	authenticatorMock.AssertExpectations(t)
}

func Test_AuthManager_UpdatePassword(t *testing.T) {
	authenticatorMock := &AuthenticatorMock{}
	jwtManagerMock := &JWTManagerMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock), WithCustomAuthenticatorOption(authenticatorMock))

	ctx := context.Background()

	user := &User{
		ID:        "user-id",
		FirstName: "First",
		LastName:  "Last",
		Email:     "email@email.com",
	}

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		jwtManagerMock.
			On("ValidateToken", ctx, "token").
			Return(false, errUnexpectedError).
			Once()

		err := authManager.UpdatePassword(ctx, "token", "currentpassword", "newpassword")

		assert.EqualError(t, err, "validating token: validating token: unexpected error")

		jwtManagerMock.
			On("ValidateToken", ctx, "token").
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, "token").
			Return(nil, errUnexpectedError).
			Once()

		err = authManager.UpdatePassword(ctx, "token", "currentpassword", "newpassword")

		assert.EqualError(t, err, "getting user from token: unexpected error")
	})

	t.Run("returns error when token is invalid", func(t *testing.T) {
		jwtManagerMock.
			On("ValidateToken", ctx, "token").
			Return(false, nil).
			Once()

		err := authManager.UpdatePassword(ctx, "token", "currentpassword", "newpassword")

		assert.EqualError(t, err, "invalid token")
	})

	t.Run("returns error when GetUserFromToken fails", func(t *testing.T) {
		jwtManagerMock.
			On("ValidateToken", ctx, "token").
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, "token").
			Return(nil, errUnexpectedError).
			Once()

		err := authManager.UpdatePassword(ctx, "token", "currentpassword", "newpassword")

		assert.EqualError(t, err, "getting user from token: unexpected error")
	})

	t.Run("returns error when Authenticator fails", func(t *testing.T) {
		jwtManagerMock.
			On("ValidateToken", ctx, "token").
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, "token").
			Return(user, nil).
			Once()

		authenticatorMock.
			On("UpdatePassword", ctx, user, "currentpassword", "newpassword").
			Return(errUnexpectedError).
			Once()

		err := authManager.UpdatePassword(ctx, "token", "currentpassword", "newpassword")

		assert.EqualError(t, err, "updating password: unexpected error")
	})

	t.Run("updates password successfully", func(t *testing.T) {
		jwtManagerMock.
			On("ValidateToken", ctx, "token").
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, "token").
			Return(user, nil).
			Once()

		authenticatorMock.
			On("UpdatePassword", ctx, user, "currentpassword", "newpassword").
			Return(nil).
			Once()

		err := authManager.UpdatePassword(ctx, "token", "currentpassword", "newpassword")

		assert.Nil(t, err)
	})
}

func Test_AuthManager_GetAllUsers(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	authenticatorMock := &AuthenticatorMock{}
	authManager := NewAuthManager(WithCustomJWTManagerOption(jwtManagerMock), WithCustomAuthenticatorOption(authenticatorMock))

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		users, err := authManager.GetAllUsers(ctx, token)

		assert.Nil(t, users)
		assert.EqualError(t, err, "validating token: validating token: unexpected error")
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		users, err := authManager.GetAllUsers(ctx, token)

		assert.Nil(t, users)
		assert.EqualError(t, err, "invalid token")
	})

	t.Run("returns error when Authenticator fails", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once()

		authenticatorMock.
			On("GetAllUsers", ctx).
			Return(nil, errUnexpectedError).
			Once()

		users, err := authManager.GetAllUsers(ctx, token)

		assert.EqualError(t, err, "error getting all users: unexpected error")
		assert.Nil(t, users)
	})

	t.Run("returns users successfully", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Twice()

		authenticatorMock.
			On("GetAllUsers", ctx).
			Return([]User{}, nil).
			Once()

		users, err := authManager.GetAllUsers(ctx, token)
		require.NoError(t, err)
		assert.Empty(t, users)

		expectedUsers := []User{
			{
				ID:        "user1-ID",
				FirstName: "First",
				LastName:  "Last",
				Email:     "user1@email.com",
				IsOwner:   false,
				IsActive:  false,
				Roles:     []string{"role1"},
			},
			{
				ID:        "user2-ID",
				FirstName: "First",
				LastName:  "Last",
				Email:     "user2@email.com",
				IsOwner:   true,
				IsActive:  true,
				Roles:     []string{"role2"},
			},
		}

		authenticatorMock.
			On("GetAllUsers", ctx).
			Return(expectedUsers, nil).
			Once()

		users, err = authManager.GetAllUsers(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, expectedUsers, users)
	})

	jwtManagerMock.AssertExpectations(t)
	authenticatorMock.AssertExpectations(t)
}

func Test_AuthManager_GetUser(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	authenticatorMock := &AuthenticatorMock{}
	roleManagerMock := &RoleManagerMock{}
	authManager := NewAuthManager(
		WithCustomJWTManagerOption(jwtManagerMock),
		WithCustomAuthenticatorOption(authenticatorMock),
		WithCustomRoleManagerOption(roleManagerMock),
	)

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		user, err := authManager.GetUser(ctx, token)

		assert.EqualError(t, err, "getting user from token: validating token: validating token: unexpected error")
		assert.Nil(t, user)
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		user, err := authManager.GetUser(ctx, token)

		assert.EqualError(t, err, "getting user from token: invalid token")
		assert.Nil(t, user)
	})

	t.Run("returns error when JWT Manager fails getting user from token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(nil, errUnexpectedError).
			Once()

		user, err := authManager.GetUser(ctx, token)

		assert.EqualError(t, err, "getting user from token: getting user from token: unexpected error")
		assert.Nil(t, user)
	})

	t.Run("returns error when Authenticator fails", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(&User{
				ID:        "user-id",
				FirstName: "First",
				LastName:  "Last",
				Email:     "email@email.com",
			}, nil).
			Once()

		authenticatorMock.
			On("GetUser", ctx, "user-id").
			Return(nil, errUnexpectedError).
			Once()

		user, err := authManager.GetUser(ctx, token)

		assert.EqualError(t, err, "getting user ID user-id: unexpected error")
		assert.Nil(t, user)
	})

	t.Run("returns error when get user roles fails", func(t *testing.T) {
		token := "mytoken"

		u := &User{
			ID:        "user-id",
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
		}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(u, nil).
			Once()

		authenticatorMock.
			On("GetUser", ctx, u.ID).
			Return(u, nil).
			Once()

		roleManagerMock.
			On("GetUserRoles", ctx, u).
			Return(nil, errUnexpectedError).
			Once()

		user, err := authManager.GetUser(ctx, token)

		assert.EqualError(t, err, "getting user ID user-id roles: unexpected error")
		assert.Nil(t, user)
	})

	t.Run("gets user successfully", func(t *testing.T) {
		token := "mytoken"

		u := &User{
			ID:        "user-id",
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
		}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(u, nil).
			Once()

		authenticatorMock.
			On("GetUser", ctx, u.ID).
			Return(u, nil).
			Once()

		roleManagerMock.
			On("GetUserRoles", ctx, u).
			Return([]string{"role1", "role2"}, nil).
			Once()

		user, err := authManager.GetUser(ctx, token)
		require.NoError(t, err)

		assert.Equal(t, u.ID, user.ID)
		assert.Equal(t, u.FirstName, user.FirstName)
		assert.Equal(t, u.LastName, user.LastName)
		assert.Equal(t, u.Email, user.Email)
		assert.Equal(t, []string{"role1", "role2"}, user.Roles)
	})

	authenticatorMock.AssertExpectations(t)
	jwtManagerMock.AssertExpectations(t)
	roleManagerMock.AssertExpectations(t)
}

func Test_AuthManager_GetUsersByID(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	authenticatorMock := &AuthenticatorMock{}
	roleManagerMock := &RoleManagerMock{}
	authManager := NewAuthManager(
		WithCustomJWTManagerOption(jwtManagerMock),
		WithCustomAuthenticatorOption(authenticatorMock),
		WithCustomRoleManagerOption(roleManagerMock),
	)

	ctx := context.Background()

	t.Run("returns error when aunthenticator fails", func(t *testing.T) {
		userIDs := []string{"invalid-id"}
		authenticatorMock.
			On("GetUsers", ctx, userIDs).
			Return(nil, errUnexpectedError).
			Once()

		_, err := authManager.GetUsersByID(ctx, userIDs)
		require.Error(t, err)
	})

	t.Run("get users by ID successfully", func(t *testing.T) {
		expectedUsers := []*User{
			{
				ID:        "user1-ID",
				FirstName: "First",
				LastName:  "Last",
			},
			{
				ID:        "user2-ID",
				FirstName: "First",
				LastName:  "Last",
			},
		}

		userIDs := []string{expectedUsers[0].ID, expectedUsers[1].ID}
		authenticatorMock.
			On("GetUsers", ctx, userIDs).
			Return(expectedUsers, nil).
			Once()

		users, err := authManager.GetUsersByID(ctx, userIDs)
		require.NoError(t, err)
		assert.Equal(t, expectedUsers, users)
	})

	authenticatorMock.AssertExpectations(t)
}

func Test_AuthManager_GetUserID(t *testing.T) {
	jwtManagerMock := &JWTManagerMock{}
	authenticatorMock := &AuthenticatorMock{}
	roleManagerMock := &RoleManagerMock{}
	authManager := NewAuthManager(
		WithCustomJWTManagerOption(jwtManagerMock),
		WithCustomAuthenticatorOption(authenticatorMock),
		WithCustomRoleManagerOption(roleManagerMock),
	)

	ctx := context.Background()

	t.Run("returns error when JWT Manager fails validating token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, errUnexpectedError).
			Once()

		userID, err := authManager.GetUserID(ctx, token)

		require.EqualError(t, err, "validating token: validating token: unexpected error")
		require.Empty(t, userID)
	})

	t.Run("returns error when token is expired", func(t *testing.T) {
		token := "myoldtoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(false, nil).
			Once()

		userID, err := authManager.GetUserID(ctx, token)

		require.EqualError(t, err, "invalid token")
		require.Empty(t, userID)
	})

	t.Run("returns error when JWT Manager fails getting user from token", func(t *testing.T) {
		token := "mytoken"

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(nil, errUnexpectedError).
			Once()

		userID, err := authManager.GetUserID(ctx, token)

		require.EqualError(t, err, "getting user from token: unexpected error")
		require.Empty(t, userID)
	})

	t.Run("gets user ID successfully", func(t *testing.T) {
		token := "mytoken"

		u := &User{
			ID:        "user-id",
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
		}

		jwtManagerMock.
			On("ValidateToken", ctx, token).
			Return(true, nil).
			Once().
			On("GetUserFromToken", ctx, token).
			Return(u, nil).
			Once()

		userID, err := authManager.GetUserID(ctx, token)
		require.NoError(t, err)
		require.Equal(t, u.ID, userID)
	})

	authenticatorMock.AssertExpectations(t)
	jwtManagerMock.AssertExpectations(t)
	roleManagerMock.AssertExpectations(t)
}
