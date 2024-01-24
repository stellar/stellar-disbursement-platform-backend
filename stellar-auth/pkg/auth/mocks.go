package auth

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"
)

// PasswordEncrypter
type PasswordEncrypterMock struct {
	mock.Mock
}

func (em *PasswordEncrypterMock) Encrypt(ctx context.Context, password string) (string, error) {
	args := em.Called(ctx, password)
	return args.Get(0).(string), args.Error(1)
}

func (em *PasswordEncrypterMock) ComparePassword(ctx context.Context, encryptedPassword, password string) (bool, error) {
	args := em.Called(ctx, encryptedPassword, password)
	return args.Get(0).(bool), args.Error(1)
}

var _ PasswordEncrypter = (*PasswordEncrypterMock)(nil)

// JWTManager
type JWTManagerMock struct {
	mock.Mock
}

func (m *JWTManagerMock) GenerateToken(ctx context.Context, user *User, expiresAt time.Time) (string, error) {
	args := m.Called(ctx, user, expiresAt)
	return args.Get(0).(string), args.Error(1)
}

func (m *JWTManagerMock) RefreshToken(ctx context.Context, token string, expiresAt time.Time) (string, error) {
	args := m.Called(ctx, token, expiresAt)
	return args.Get(0).(string), args.Error(1)
}

func (m *JWTManagerMock) ValidateToken(ctx context.Context, token string) (bool, error) {
	args := m.Called(ctx, token)
	return args.Get(0).(bool), args.Error(1)
}

func (m *JWTManagerMock) GetUserFromToken(ctx context.Context, tokenString string) (*User, error) {
	args := m.Called(ctx, tokenString)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

var _ JWTManager = (*JWTManagerMock)(nil)

// Authenticator
type AuthenticatorMock struct {
	mock.Mock
}

func (am *AuthenticatorMock) ValidateCredentials(ctx context.Context, email, password string) (*User, error) {
	args := am.Called(ctx, email, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (am *AuthenticatorMock) CreateUser(ctx context.Context, user *User, password string) (*User, error) {
	args := am.Called(ctx, user, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (am *AuthenticatorMock) UpdateUser(ctx context.Context, ID, firstName, lastName, email, password string) error {
	args := am.Called(ctx, ID, firstName, lastName, email, password)
	return args.Error(0)
}

func (am *AuthenticatorMock) ActivateUser(ctx context.Context, userID string) error {
	args := am.Called(ctx, userID)
	return args.Error(0)
}

func (am *AuthenticatorMock) DeactivateUser(ctx context.Context, userID string) error {
	args := am.Called(ctx, userID)
	return args.Error(0)
}

func (am *AuthenticatorMock) ForgotPassword(ctx context.Context, email string) (string, error) {
	args := am.Called(ctx, email)
	return args.Get(0).(string), args.Error(1)
}

func (am *AuthenticatorMock) ResetPassword(ctx context.Context, resetToken, password string) error {
	args := am.Called(ctx, resetToken, password)
	return args.Error(0)
}

func (am *AuthenticatorMock) UpdatePassword(ctx context.Context, user *User, currentPassword, newPassword string) error {
	args := am.Called(ctx, user, currentPassword, newPassword)
	return args.Error(0)
}

func (am *AuthenticatorMock) GetAllUsers(ctx context.Context) ([]User, error) {
	args := am.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]User), args.Error(1)
}

func (am *AuthenticatorMock) GetUser(ctx context.Context, userID string) (*User, error) {
	args := am.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (am *AuthenticatorMock) GetUsers(ctx context.Context, userIDs []string) ([]*User, error) {
	args := am.Called(ctx, userIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*User), args.Error(1)
}

var _ Authenticator = (*AuthenticatorMock)(nil)

type RoleManagerMock struct {
	mock.Mock
}

func (rm *RoleManagerMock) GetUserRoles(ctx context.Context, user *User) ([]string, error) {
	args := rm.Called(ctx, user)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (rm *RoleManagerMock) HasAllRoles(ctx context.Context, user *User, roleNames []string) (bool, error) {
	args := rm.Called(ctx, user, roleNames)
	return args.Get(0).(bool), args.Error(1)
}

func (rm *RoleManagerMock) HasAnyRoles(ctx context.Context, user *User, roleNames []string) (bool, error) {
	args := rm.Called(ctx, user, roleNames)
	return args.Get(0).(bool), args.Error(1)
}

func (rm *RoleManagerMock) IsSuperUser(ctx context.Context, user *User) (bool, error) {
	args := rm.Called(ctx, user)
	return args.Get(0).(bool), args.Error(1)
}

func (rm *RoleManagerMock) UpdateRoles(ctx context.Context, user *User, roleNames []string) error {
	args := rm.Called(ctx, user, roleNames)
	return args.Error(0)
}

var _ RoleManager = (*RoleManagerMock)(nil)

// MFAManager
type MFAManagerMock struct {
	mock.Mock
}

func (m *MFAManagerMock) MFADeviceRemembered(ctx context.Context, deviceID, userID string) (bool, error) {
	args := m.Called(ctx, deviceID, userID)
	return args.Get(0).(bool), args.Error(1)
}

func (m *MFAManagerMock) GenerateMFACode(ctx context.Context, deviceID, userID string) (string, error) {
	args := m.Called(ctx, deviceID, userID)
	return args.Get(0).(string), args.Error(1)
}

func (m *MFAManagerMock) ValidateMFACode(ctx context.Context, deviceID, code string) (string, error) {
	args := m.Called(ctx, deviceID, code)
	return args.Get(0).(string), args.Error(1)
}

func (m *MFAManagerMock) RememberDevice(ctx context.Context, deviceID, code string) error {
	args := m.Called(ctx, deviceID, code)
	return args.Error(0)
}

var _ MFAManager = (*MFAManagerMock)(nil)

// AuthManager
type AuthManagerMock struct {
	mock.Mock
}

func (am *AuthManagerMock) Authenticate(ctx context.Context, email, pass string) (string, error) {
	args := am.Called(ctx, email, pass)
	return args.Get(0).(string), args.Error(1)
}

func (am *AuthManagerMock) RefreshToken(ctx context.Context, tokenString string) (string, error) {
	args := am.Called(ctx, tokenString)
	return args.Get(0).(string), args.Error(1)
}

func (am *AuthManagerMock) ValidateToken(ctx context.Context, tokenString string) (bool, error) {
	args := am.Called(ctx, tokenString)
	return args.Get(0).(bool), args.Error(1)
}

func (am *AuthManagerMock) AllRolesInTokenUser(ctx context.Context, tokenString string, roleNames []string) (bool, error) {
	args := am.Called(ctx, tokenString, roleNames)
	return args.Get(0).(bool), args.Error(1)
}

func (am *AuthManagerMock) AnyRolesInTokenUser(ctx context.Context, tokenString string, roleNames []string) (bool, error) {
	args := am.Called(ctx, tokenString, roleNames)
	return args.Get(0).(bool), args.Error(1)
}

func (am *AuthManagerMock) CreateUser(ctx context.Context, user *User, password string) (*User, error) {
	args := am.Called(ctx, user, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (am *AuthManagerMock) UpdateUser(ctx context.Context, tokenString, firstName, lastName, email, password string) error {
	args := am.Called(ctx, tokenString, firstName, lastName, email, password)
	return args.Error(0)
}

func (am *AuthManagerMock) ForgotPassword(ctx context.Context, email string) (string, error) {
	args := am.Called(ctx, email)
	return args.Get(0).(string), args.Error(1)
}

func (am *AuthManagerMock) ResetPassword(ctx context.Context, tokenString, password string) error {
	args := am.Called(ctx, tokenString, password)
	return args.Error(0)
}

func (am *AuthManagerMock) UpdatePassword(ctx context.Context, token, currentPassword, newPassword string) error {
	args := am.Called(ctx, token, currentPassword, newPassword)
	return args.Error(0)
}

func (am *AuthManagerMock) GetUser(ctx context.Context, tokenString string) (*User, error) {
	args := am.Called(ctx, tokenString)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (am *AuthManagerMock) GetUsersByID(ctx context.Context, tokenString []string) ([]*User, error) {
	args := am.Called(ctx, tokenString)
	return args.Get(0).([]*User), args.Error(1)
}

func (am *AuthManagerMock) GetUserID(ctx context.Context, userID string) (string, error) {
	args := am.Called(ctx, userID)
	return args.Get(0).(string), args.Error(1)
}

func (am *AuthManagerMock) GetAllUsers(ctx context.Context, tokenString string) ([]User, error) {
	args := am.Called(ctx, tokenString)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]User), args.Error(1)
}

func (am *AuthManagerMock) UpdateUserRoles(ctx context.Context, tokenString, userID string, roles []string) error {
	args := am.Called(ctx, tokenString, userID, roles)
	return args.Error(0)
}

func (am *AuthManagerMock) DeactivateUser(ctx context.Context, tokenString, userID string) error {
	args := am.Called(ctx, tokenString, userID)
	return args.Error(0)
}

func (am *AuthManagerMock) ActivateUser(ctx context.Context, tokenString, userID string) error {
	args := am.Called(ctx, tokenString, userID)
	return args.Error(0)
}

func (am *AuthManagerMock) ExpirationTimeInMinutes() time.Duration {
	args := am.Called()
	return args.Get(0).(time.Duration)
}

func (am *AuthManagerMock) MFADeviceRemembered(ctx context.Context, userID, deviceID string) (bool, error) {
	args := am.Called(ctx, userID, deviceID)
	return args.Get(0).(bool), args.Error(1)
}

func (am *AuthManagerMock) GetMFACode(ctx context.Context, userID, deviceID string) (string, error) {
	args := am.Called(ctx, userID, deviceID)
	return args.Get(0).(string), args.Error(1)
}

func (am *AuthManagerMock) GenerateMFACode(ctx context.Context, userID, deviceID string) (string, error) {
	args := am.Called(ctx, userID, deviceID)
	return args.Get(0).(string), args.Error(1)
}

func (am *AuthManagerMock) AuthenticateMFA(ctx context.Context, deviceID, code string, rememberMe bool) (string, error) {
	args := am.Called(ctx, deviceID, code, rememberMe)
	return args.Get(0).(string), args.Error(1)
}

var _ AuthManager = (*AuthManagerMock)(nil)
