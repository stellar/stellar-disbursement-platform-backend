package auth

import (
	"fmt"
	"time"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

const defaultExpirationTimeInMinutes = 15

type User struct {
	ID        string   `json:"id"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Email     string   `json:"email"`
	IsOwner   bool     `json:"-"`
	IsActive  bool     `json:"is_active"`
	Roles     []string `json:"roles"`
}

func (u *User) Validate() error {
	if u.Email == "" {
		return fmt.Errorf("email is required")
	} else if err := utils.ValidateEmail(u.Email); err != nil {
		return fmt.Errorf("email is invalid: %w", err)
	}

	if u.FirstName == "" {
		return fmt.Errorf("first name is required")
	}

	if u.LastName == "" {
		return fmt.Errorf("last name is required")
	}

	return nil
}

// AuthManager manages the JWT token generation, validation and refresh. Use `NewAuthManager` function
// to construct a new pointer.
type defaultAuthManager struct {
	expirationTimeInMinutes time.Duration
	authenticator           Authenticator
	jwtManager              JWTManager
	roleManager             RoleManager
	mfaManager              MFAManager
}

type AuthManagerOption func(am *defaultAuthManager)

// NewAuthManager constructs a new `*AuthManager` and apply the options passed by parameter.
func NewAuthManager(options ...AuthManagerOption) AuthManager {
	authManager := &defaultAuthManager{
		expirationTimeInMinutes: time.Minute * defaultExpirationTimeInMinutes,
	}

	for _, option := range options {
		option(authManager)
	}

	return authManager
}

// WithDefaultAuthenticatorOption sets a default authentication method that validates the users' credentials.
func WithDefaultAuthenticatorOption(dbConnectionPool db.DBConnectionPool, passwordEncrypter PasswordEncrypter, resetTokenExpirationHours time.Duration) AuthManagerOption {
	return func(am *defaultAuthManager) {
		am.authenticator = newDefaultAuthenticator(
			withAuthenticatorDatabaseConnectionPool(dbConnectionPool),
			withPasswordEncrypter(passwordEncrypter),
			withResetTokenExpirationHours(resetTokenExpirationHours),
		)
	}
}

// WithDefaultAuthenticatorOption sets a custom authentication method that implements the `Authenticator` interface.
func WithCustomAuthenticatorOption(authenticator Authenticator) AuthManagerOption {
	return func(am *defaultAuthManager) {
		am.authenticator = authenticator
	}
}

// WithDefaultJWTManagerOption sets a default JWT Manager that generates, validates and refreshes the users' JWT token.
func WithDefaultJWTManagerOption(ECPublicKey, ECPrivateKey string) AuthManagerOption {
	return func(am *defaultAuthManager) {
		am.jwtManager = newDefaultJWTManager(withECKeypair(ECPublicKey, ECPrivateKey))
	}
}

// WithDefaultJWTManagerOption sets a custom JWT Manager that implements the `JWTManager` interface.
func WithCustomJWTManagerOption(jwtManager JWTManager) AuthManagerOption {
	return func(am *defaultAuthManager) {
		am.jwtManager = jwtManager
	}
}

// WithExpirationTimeInMinutesOption sets the JWT token expiration time in minutes. Default is `15 minutes`.
func WithExpirationTimeInMinutesOption(minutes int) AuthManagerOption {
	return func(am *defaultAuthManager) {
		am.expirationTimeInMinutes = time.Minute * time.Duration(minutes)
	}
}

func WithDefaultRoleManagerOption(dbConnectionPool db.DBConnectionPool, ownerRoleName string) AuthManagerOption {
	return func(am *defaultAuthManager) {
		roleOptions := []defaultRoleManagerOption{
			withRoleManagerDBConnectionPool(dbConnectionPool),
		}

		if ownerRoleName != "" {
			roleOptions = append(roleOptions, withOwnerRoleName(ownerRoleName))
		}

		am.roleManager = newDefaultRoleManager(roleOptions...)
	}
}

func WithCustomRoleManagerOption(roleManager RoleManager) AuthManagerOption {
	return func(am *defaultAuthManager) {
		am.roleManager = roleManager
	}
}

func WithDefaultMFAManagerOption(dbConnectionPool db.DBConnectionPool) AuthManagerOption {
	return func(am *defaultAuthManager) {
		am.mfaManager = newDefaultMFAManager(withMFADatabaseConnectionPool(dbConnectionPool))
	}
}

func WithCustomMFAManagerOption(mfaManager MFAManager) AuthManagerOption {
	return func(am *defaultAuthManager) {
		am.mfaManager = mfaManager
	}
}
