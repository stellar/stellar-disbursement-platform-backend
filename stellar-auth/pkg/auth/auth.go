package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
)

var ErrInvalidToken = errors.New("invalid token")

type AuthManager interface {
	Authenticate(ctx context.Context, email, pass string) (string, error)
	RefreshToken(ctx context.Context, tokenString string) (string, error)
	ValidateToken(ctx context.Context, tokenString string) (bool, error)
	AllRolesInTokenUser(ctx context.Context, tokenString string, roleNames []string) (bool, error)
	AnyRolesInTokenUser(ctx context.Context, tokenString string, roleNames []string) (bool, error)
	CreateUser(ctx context.Context, user *User, password string) (*User, error)
	UpdateUser(ctx context.Context, tokenString, firstName, lastName, email, password string) error
	ForgotPassword(ctx context.Context, email string) (string, error)
	ResetPassword(ctx context.Context, tokenString, password string) error
	UpdatePassword(ctx context.Context, token, currentPassword, newPassword string) error
	GetUser(ctx context.Context, tokenString string) (*User, error)
	GetUsersByID(ctx context.Context, userIDs []string) ([]*User, error)
	GetUserID(ctx context.Context, tokenString string) (string, error)
	GetAllUsers(ctx context.Context, tokenString string) ([]User, error)
	UpdateUserRoles(ctx context.Context, tokenString, userID string, roles []string) error
	DeactivateUser(ctx context.Context, tokenString, userID string) error
	ActivateUser(ctx context.Context, tokenString, userID string) error
	ExpirationTimeInMinutes() time.Duration
	MFADeviceRemembered(ctx context.Context, deviceID, userID string) (bool, error)
	GetMFACode(ctx context.Context, deviceID, userID string) (string, error)
	AuthenticateMFA(ctx context.Context, deviceID, code string, rememberMe bool) (string, error)
}

// DBConnectionPoolFromSqlDB returns a new DBConnectionPool wrapper for a PRE-EXISTING *sql.DB. The driverName of the
// original database is required for named query support. ATTENTION: this will not start a new connection pool, just
// create a wrap aroung the pre-existing connection pool.
func DBConnectionPoolFromSqlDB(sqlDB *sql.DB, driverName string) db.DBConnectionPool {
	return db.DBConnectionPoolFromSqlDB(sqlDB, driverName)
}

func (am *defaultAuthManager) Authenticate(ctx context.Context, email, pass string) (string, error) {
	user, err := am.authenticator.ValidateCredentials(ctx, email, pass)
	if err != nil {
		return "", fmt.Errorf("validating credentials: %w", err)
	}

	return am.generateToken(ctx, user)
}

func (am *defaultAuthManager) generateToken(ctx context.Context, user *User) (string, error) {
	roles, err := am.roleManager.GetUserRoles(ctx, user)
	if err != nil {
		return "", fmt.Errorf("error getting user roles: %w", err)
	}

	user.Roles = roles

	expiresAt := time.Now().Add(am.expirationTimeInMinutes)
	tokenString, err := am.jwtManager.GenerateToken(ctx, user, expiresAt)
	if err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}

	return tokenString, nil
}

func (am *defaultAuthManager) RefreshToken(ctx context.Context, tokenString string) (string, error) {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return "", fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return "", ErrInvalidToken
	}

	// TODO: find a way to not refresh the same token
	// more than once - perhaps create a table and store invalid tokens
	expiresAt := time.Now().Add(am.expirationTimeInMinutes)
	tokenString, err = am.jwtManager.RefreshToken(ctx, tokenString, expiresAt)
	if err != nil {
		return "", fmt.Errorf("generating new refreshed token: %w", err)
	}

	return tokenString, nil
}

func (am *defaultAuthManager) ValidateToken(ctx context.Context, tokenString string) (bool, error) {
	isValid, err := am.jwtManager.ValidateToken(ctx, tokenString)
	if err != nil {
		return false, fmt.Errorf("validating token: %w", err)
	}

	return isValid, nil
}

// AllRolesInTokenUser checks whether the user's token has all the roles passed by parameter.
func (am *defaultAuthManager) AllRolesInTokenUser(ctx context.Context, tokenString string, roleNames []string) (bool, error) {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return false, fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return false, ErrInvalidToken
	}

	user, err := am.jwtManager.GetUserFromToken(ctx, tokenString)
	if err != nil {
		return false, fmt.Errorf("error getting user from token: %w", err)
	}

	hasAllRoles, err := am.roleManager.HasAllRoles(ctx, user, roleNames)
	if err != nil {
		return false, fmt.Errorf("error validating user roles: %w", err)
	}

	return hasAllRoles, nil
}

// AnyRolesInTokenUser checks whether the user's token has one or more the roles passed by parameter.
func (am *defaultAuthManager) AnyRolesInTokenUser(ctx context.Context, tokenString string, roleNames []string) (bool, error) {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return false, fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return false, ErrInvalidToken
	}

	user, err := am.jwtManager.GetUserFromToken(ctx, tokenString)
	if err != nil {
		return false, fmt.Errorf("error getting user from token: %w", err)
	}

	hasAnyRoles, err := am.roleManager.HasAnyRoles(ctx, user, roleNames)
	if err != nil {
		return false, fmt.Errorf("error validating user roles: %w", err)
	}

	return hasAnyRoles, nil
}

// CreateUser creates a new user using Authenticator's CreateUser method.
func (am *defaultAuthManager) CreateUser(ctx context.Context, user *User, password string) (*User, error) {
	user, err := am.authenticator.CreateUser(ctx, user, password)
	if err != nil {
		return nil, fmt.Errorf("error creating user: %w", err)
	}

	return user, nil
}

func (am *defaultAuthManager) UpdateUser(ctx context.Context, tokenString, firstName, lastName, email, password string) error {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return ErrInvalidToken
	}

	user, err := am.jwtManager.GetUserFromToken(ctx, tokenString)
	if err != nil {
		return fmt.Errorf("getting user from token: %w", err)
	}

	err = am.authenticator.UpdateUser(ctx, user.ID, firstName, lastName, email, password)
	if err != nil {
		return fmt.Errorf("error updating user: %w", err)
	}

	return nil
}

// ForgotPassword handles the generation of a new password reset token for the user to set a new password.
func (am *defaultAuthManager) ForgotPassword(ctx context.Context, email string) (string, error) {
	resetToken, err := am.authenticator.ForgotPassword(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", fmt.Errorf("user not found in auth forgot password: %w", err)
		}
		return "", fmt.Errorf("error on forgot password: %w", err)
	}

	return resetToken, nil
}

// ResetPassword sets the user's new password using a valid reset token generated in the ForgotPassword flow.
func (am *defaultAuthManager) ResetPassword(ctx context.Context, resetToken, newPassword string) error {
	err := am.authenticator.ResetPassword(ctx, resetToken, newPassword)
	if err != nil {
		if errors.Is(err, ErrInvalidResetPasswordToken) {
			return fmt.Errorf("invalid token in auth reset password: %w", err)
		}
		return fmt.Errorf("error on reset password: %w", err)
	}

	return nil
}

func (am *defaultAuthManager) UpdatePassword(ctx context.Context, tokenString, currentPassword, newPassword string) error {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return ErrInvalidToken
	}

	user, err := am.jwtManager.GetUserFromToken(ctx, tokenString)
	if err != nil {
		return fmt.Errorf("getting user from token: %w", err)
	}

	err = am.authenticator.UpdatePassword(ctx, user, currentPassword, newPassword)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	return nil
}

func (am *defaultAuthManager) ActivateUser(ctx context.Context, tokenString, userID string) error {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return ErrInvalidToken
	}

	err = am.authenticator.ActivateUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("error activating user ID %s: %w", userID, err)
	}

	return nil
}

func (am *defaultAuthManager) DeactivateUser(ctx context.Context, tokenString, userID string) error {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return ErrInvalidToken
	}

	err = am.authenticator.DeactivateUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("error deactivating user ID %s: %w", userID, err)
	}

	return nil
}

func (am *defaultAuthManager) UpdateUserRoles(ctx context.Context, tokenString, userID string, roles []string) error {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return ErrInvalidToken
	}

	// TODO: pass all fields of the user
	err = am.roleManager.UpdateRoles(ctx, &User{ID: userID}, roles)
	if err != nil {
		return fmt.Errorf("error updating user roles: %w", err)
	}

	return nil
}

func (am *defaultAuthManager) GetAllUsers(ctx context.Context, tokenString string) ([]User, error) {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return nil, fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return nil, ErrInvalidToken
	}

	users, err := am.authenticator.GetAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting all users: %w", err)
	}

	return users, nil
}

func (am *defaultAuthManager) getUserFromToken(ctx context.Context, tokenString string) (*User, error) {
	isValid, err := am.ValidateToken(ctx, tokenString)
	if err != nil {
		return nil, fmt.Errorf("validating token: %w", err)
	}

	if !isValid {
		return nil, ErrInvalidToken
	}

	user, err := am.jwtManager.GetUserFromToken(ctx, tokenString)
	if err != nil {
		return nil, fmt.Errorf("getting user from token: %w", err)
	}

	return user, nil
}

func (am *defaultAuthManager) GetUsersByID(ctx context.Context, userIDs []string) ([]*User, error) {
	users, err := am.authenticator.GetUsers(ctx, userIDs)
	if err != nil {
		return nil, fmt.Errorf("getting user with IDs: %w", err)
	}

	return users, nil
}

func (am *defaultAuthManager) GetUserID(ctx context.Context, tokenString string) (string, error) {
	tokenUser, err := am.getUserFromToken(ctx, tokenString)
	if err != nil {
		return "", err
	}

	return tokenUser.ID, nil
}

func (am *defaultAuthManager) GetUser(ctx context.Context, tokenString string) (*User, error) {
	tokenUser, err := am.getUserFromToken(ctx, tokenString)
	if err != nil {
		return nil, fmt.Errorf("getting user from token: %w", err)
	}

	// We get the user latest state
	user, err := am.authenticator.GetUser(ctx, tokenUser.ID)
	if err != nil {
		return nil, fmt.Errorf("getting user ID %s: %w", tokenUser.ID, err)
	}

	roles, err := am.roleManager.GetUserRoles(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("getting user ID %s roles: %w", tokenUser.ID, err)
	}

	user.Roles = roles

	return user, nil
}

func (am *defaultAuthManager) ExpirationTimeInMinutes() time.Duration {
	return am.expirationTimeInMinutes
}

func (am *defaultAuthManager) MFADeviceRemembered(ctx context.Context, deviceID, userID string) (bool, error) {
	return am.mfaManager.MFADeviceRemembered(ctx, deviceID, userID)
}

func (am *defaultAuthManager) GetMFACode(ctx context.Context, deviceID, userID string) (string, error) {
	return am.mfaManager.GenerateMFACode(ctx, deviceID, userID)
}

func (am *defaultAuthManager) AuthenticateMFA(ctx context.Context, deviceID, code string, rememberMe bool) (string, error) {
	if rememberMe {
		err := am.mfaManager.RememberDevice(ctx, deviceID, code)
		if err != nil {
			return "", fmt.Errorf("error remembering device ID %s: %w", deviceID, err)
		}
	}

	userID, err := am.mfaManager.ValidateMFACode(ctx, deviceID, code)
	if err != nil {
		return "", fmt.Errorf("error validating MFA code: %w", err)
	}

	user, err := am.authenticator.GetUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("error getting user ID %s: %w", userID, err)
	}

	return am.generateToken(ctx, user)
}

// Ensuring that defaultAuthManager is implementing AuthManager interface
var _ AuthManager = (*defaultAuthManager)(nil)
