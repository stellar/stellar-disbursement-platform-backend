package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
)

var (
	ErrInvalidCredentials        = errors.New("invalid credentials")
	ErrNoRowsAffected            = errors.New("no rows affected")
	ErrInvalidResetPasswordToken = errors.New("invalid reset password token")
	ErrUserNotFound              = errors.New("user not found")
	ErrUserEmailAlreadyExists    = errors.New("a user with this email already exists")
	ErrUserHasValidToken         = errors.New("user has a valid token")
)

const (
	resetTokenLength = 10
)

type Authenticator interface {
	ValidateCredentials(ctx context.Context, email, password string) (*User, error)
	// CreateUser creates a new user it receives a user object and the password
	CreateUser(ctx context.Context, user *User, password string) (*User, error)
	UpdateUser(ctx context.Context, ID, firstName, lastName, email, password string) error
	ActivateUser(ctx context.Context, userID string) error
	DeactivateUser(ctx context.Context, userID string) error
	ForgotPassword(ctx context.Context, email string) (string, error)
	ResetPassword(ctx context.Context, resetToken, password string) error
	UpdatePassword(ctx context.Context, user *User, currentPassword, newPassword string) error
	GetAllUsers(ctx context.Context) ([]User, error)
	GetUser(ctx context.Context, userID string) (*User, error)
	GetUsers(ctx context.Context, userIDs []string) ([]*User, error)
}

type defaultAuthenticator struct {
	dbConnectionPool          db.DBConnectionPool
	passwordEncrypter         PasswordEncrypter
	resetTokenExpirationHours time.Duration
}

type authUser struct {
	ID                string `db:"id"`
	FirstName         string `db:"first_name"`
	LastName          string `db:"last_name"`
	Email             string `db:"email"`
	EncryptedPassword string `db:"encrypted_password"`
}

func (a *defaultAuthenticator) ValidateCredentials(ctx context.Context, email, password string) (*User, error) {
	const query = `
		SELECT
			u.id,
			u.first_name,
			u.last_name,
			u.encrypted_password
		FROM
			auth_users u
		WHERE
			email = $1 AND is_active = true
	`

	au := authUser{}
	err := a.dbConnectionPool.GetContext(ctx, &au, query, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}

		return nil, fmt.Errorf("querying user: %w", err)
	}

	isEqual, err := a.passwordEncrypter.ComparePassword(ctx, au.EncryptedPassword, password)
	if err != nil {
		return nil, fmt.Errorf("comparing password: %w", err)
	}
	if !isEqual {
		return nil, ErrInvalidCredentials
	}

	return &User{
		ID:        au.ID,
		Email:     email,
		FirstName: au.FirstName,
		LastName:  au.LastName,
	}, nil
}

// CreateUser creates a user in the database. If a empty password is passed by parameter, a random password is generated,
// so the user can go through the ForgotPassword flow.
func (a *defaultAuthenticator) CreateUser(ctx context.Context, user *User, password string) (*User, error) {
	if err := user.Validate(); err != nil {
		return nil, fmt.Errorf("error validating user fields: %w", err)
	}

	// In case no password is passed we generate a random OTP (One Time Password)
	if password == "" {
		// Random length pasword
		randomNumber, err := rand.Int(rand.Reader, big.NewInt(MaxPasswordLength-MinPasswordLength+1))
		if err != nil {
			return nil, fmt.Errorf("error generating random number in create user: %w", err)
		}

		passwordLength := int(randomNumber.Int64() + MinPasswordLength)
		password, err = utils.StringWithCharset(passwordLength, utils.PasswordCharset)
		if err != nil {
			return nil, fmt.Errorf("error generating random password string in create user: %w", err)
		}
	}

	encryptedPassword, err := a.passwordEncrypter.Encrypt(ctx, password)
	if err != nil {
		return nil, fmt.Errorf("error encrypting password: %w", err)
	}

	const query = `
		INSERT INTO auth_users
			(email, encrypted_password, first_name, last_name, roles, is_owner)
		VALUES
			($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	var userID string
	err = a.dbConnectionPool.GetContext(ctx, &userID, query, user.Email, encryptedPassword, user.FirstName, user.LastName, pq.Array(user.Roles), user.IsOwner)
	if err != nil {
		if pqError, ok := err.(*pq.Error); ok && pqError.Constraint == "auth_users_email_key" {
			return nil, ErrUserEmailAlreadyExists
		}
		return nil, fmt.Errorf("error inserting user: %w", err)
	}

	user.ID = userID
	user.IsActive = true

	return user, nil
}

func (a *defaultAuthenticator) UpdateUser(ctx context.Context, ID, firstName, lastName, email, password string) error {
	if firstName == "" && lastName == "" && email == "" && password == "" {
		return fmt.Errorf("provide at least one of these values: firstName, lastName, email or password")
	}

	query := `
		UPDATE
			auth_users
		SET
			%s
		WHERE id = ?
	`

	fields := []string{}
	args := []interface{}{}
	if firstName != "" {
		fields = append(fields, "first_name = ?")
		args = append(args, firstName)
	}

	if lastName != "" {
		fields = append(fields, "last_name = ?")
		args = append(args, lastName)
	}

	if email != "" {
		if err := utils.ValidateEmail(email); err != nil {
			return fmt.Errorf("error validating email: %w", err)
		}

		fields = append(fields, "email = ?")
		args = append(args, email)
	}

	if password != "" {
		encryptedPassword, err := a.passwordEncrypter.Encrypt(ctx, password)
		if err != nil {
			if !errors.Is(err, ErrPasswordTooShort) {
				return fmt.Errorf("error encrypting password: %w", err)
			}
			return err
		}

		fields = append(fields, "encrypted_password = ?")
		args = append(args, encryptedPassword)
	}

	query = a.dbConnectionPool.Rebind(fmt.Sprintf(query, strings.Join(fields, ", ")))
	args = append(args, ID)

	res, err := a.dbConnectionPool.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("error updating user in the database: %w", err)
	}

	numRowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting the number of rows affected: %w", err)
	}
	if numRowsAffected == 0 {
		return ErrNoRowsAffected
	}

	return nil
}

func (a *defaultAuthenticator) updateIsActive(ctx context.Context, userID string, isActive bool) error {
	const query = "UPDATE auth_users SET is_active = $1 WHERE id = $2"

	result, err := a.dbConnectionPool.ExecContext(ctx, query, isActive, userID)
	if err != nil {
		return fmt.Errorf("error updating is_active for user ID %s: %w", userID, err)
	}

	numRowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting number of rows affected: %w", err)
	}

	if numRowsAffected == 0 {
		return ErrNoRowsAffected
	}

	return nil
}

func (a *defaultAuthenticator) ActivateUser(ctx context.Context, userID string) error {
	err := a.updateIsActive(ctx, userID, true)
	if err != nil {
		return fmt.Errorf("error activating user ID %s: %w", userID, err)
	}

	return nil
}

func (a *defaultAuthenticator) DeactivateUser(ctx context.Context, userID string) error {
	err := a.updateIsActive(ctx, userID, false)
	if err != nil {
		return fmt.Errorf("error deactivating user ID %s: %w", userID, err)
	}

	return nil
}

func (a *defaultAuthenticator) ForgotPassword(ctx context.Context, email string) (string, error) {
	if email == "" {
		return "", fmt.Errorf("error generating user reset password token: email cannot be empty")
	}

	resetToken, err := utils.StringWithCharset(resetTokenLength, utils.DefaultCharset)
	if err != nil {
		return "", fmt.Errorf("error generating random reset token in forgot password: %w", err)
	}

	checkValidTokenQuery := `
		SELECT EXISTS (
			SELECT 1
			FROM auth_user_password_reset ar
			INNER JOIN auth_users au ON ar.auth_user_id = au.id
			WHERE au.email = $1
			AND ar.is_valid = true
			AND (ar.created_at + INTERVAL '20 minutes') > now()
		)
	`
	var hasValidToken bool
	err = a.dbConnectionPool.GetContext(ctx, &hasValidToken, checkValidTokenQuery, email)
	if err != nil {
		return "", fmt.Errorf("error checking if user has valid token: %w", err)
	}

	if hasValidToken {
		return "", ErrUserHasValidToken
	}

	q := `
		WITH auth_user_reset_token_info AS (
			SELECT id, $2 as reset_token FROM auth_users WHERE email = $1
		)
		INSERT INTO
			auth_user_password_reset (auth_user_id, token)
			SELECT id, reset_token FROM auth_user_reset_token_info
	`
	result, err := a.dbConnectionPool.ExecContext(ctx, q, email, resetToken)
	if err != nil {
		return "", fmt.Errorf("error inserting user reset password token in the database: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("error getting rows affected inserting user reset password token in the database: %w", err)
	}
	if rowsAffected == 0 {
		return "", ErrUserNotFound
	}

	return resetToken, nil
}

func (a *defaultAuthenticator) ResetPassword(ctx context.Context, resetToken, password string) error {
	return db.RunInTransaction(ctx, a.dbConnectionPool, nil, func(dbTx db.DBTransaction) error {
		query := `
			SELECT
				auth_user_id, created_at
			FROM
				auth_user_password_reset
			WHERE
				token = $1 AND is_valid = true
		`

		type authUserPasswordReset struct {
			UserID    string    `db:"auth_user_id"`
			CreatedAt time.Time `db:"created_at"`
		}

		var aupr authUserPasswordReset
		err := dbTx.GetContext(ctx, &aupr, query, resetToken)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidResetPasswordToken
			}
			return fmt.Errorf("error searching password reset token for user in database: %w", err)
		}

		// Token is only valid for 20 minutes
		if aupr.CreatedAt.Add(time.Minute * 20).Before(time.Now()) {
			return ErrInvalidResetPasswordToken
		}

		encryptedPassword, err := a.passwordEncrypter.Encrypt(ctx, password)
		if err != nil {
			return fmt.Errorf("error trying to encrypt user password: %w", err)
		}

		query = `UPDATE auth_users SET encrypted_password = $1 WHERE id = $2`
		_, err = dbTx.ExecContext(ctx, query, encryptedPassword, aupr.UserID)
		if err != nil {
			return fmt.Errorf("error reseting user password in the database: %w", err)
		}

		err = a.invalidateResetPasswordToken(ctx, dbTx, resetToken)
		if err != nil {
			return fmt.Errorf("error invalidating reset password token: %w", err)
		}

		return nil
	})
}

func (a *defaultAuthenticator) UpdatePassword(ctx context.Context, user *User, currentPassword, newPassword string) error {
	if currentPassword == "" || newPassword == "" {
		return fmt.Errorf("provide currentPassword and newPassword values")
	}

	_, err := a.ValidateCredentials(ctx, user.Email, currentPassword)
	if err != nil {
		return fmt.Errorf("validating credentials: %w", err)
	}

	query := `
		UPDATE
			auth_users
		SET
			encrypted_password = $1
		WHERE id = $2
	`

	encryptedPassword, err := a.passwordEncrypter.Encrypt(ctx, newPassword)
	if err != nil {
		if !errors.Is(err, ErrPasswordTooShort) {
			return fmt.Errorf("encrypting password: %w", err)
		}
		return err
	}

	res, err := a.dbConnectionPool.ExecContext(ctx, query, encryptedPassword, user.ID)
	if err != nil {
		return fmt.Errorf("updating user password in the database: %w", err)
	}

	numRowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting the number of rows affected: %w", err)
	}
	if numRowsAffected == 0 {
		return ErrNoRowsAffected
	}

	return nil
}

func (a *defaultAuthenticator) invalidateResetPasswordToken(ctx context.Context, dbTx db.DBTransaction, resetToken string) error {
	q := "UPDATE auth_user_password_reset SET is_valid = false WHERE token = $1"
	_, err := dbTx.ExecContext(ctx, q, resetToken)
	if err != nil {
		return fmt.Errorf("error invalidating reset password token in the database: %w", err)
	}

	return nil
}

func (a *defaultAuthenticator) GetAllUsers(ctx context.Context) ([]User, error) {
	const query = `
		SELECT
			id,
			first_name,
			last_name,
			email,
			roles,
			is_owner,
			is_active
		FROM
			auth_users
	`

	dbUsers := []struct {
		ID        string         `db:"id"`
		FirstName string         `db:"first_name"`
		LastName  string         `db:"last_name"`
		Email     string         `db:"email"`
		Roles     pq.StringArray `db:"roles"`
		IsOwner   bool           `db:"is_owner"`
		IsActive  bool           `db:"is_active"`
	}{}
	err := a.dbConnectionPool.SelectContext(ctx, &dbUsers, query)
	if err != nil {
		return nil, fmt.Errorf("error querying all users in the database: %w", err)
	}

	users := []User{}
	for _, dbUser := range dbUsers {
		users = append(users, User{
			ID:        dbUser.ID,
			FirstName: dbUser.FirstName,
			LastName:  dbUser.LastName,
			Email:     dbUser.Email,
			IsOwner:   dbUser.IsOwner,
			IsActive:  dbUser.IsActive,
			Roles:     dbUser.Roles,
		})
	}

	return users, nil
}

func (a *defaultAuthenticator) GetUser(ctx context.Context, userID string) (*User, error) {
	const query = `
		SELECT
			first_name,
			last_name,
			email
		FROM
			auth_users
		WHERE
			id = $1 AND is_active = true
	`

	var u authUser
	err := a.dbConnectionPool.GetContext(ctx, &u, query, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("error querying user ID %s: %w", userID, err)
	}

	return &User{
		ID:        userID,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Email:     u.Email,
	}, nil
}

// GetUsers retrieves the respective users from a list of user IDs.
func (a *defaultAuthenticator) GetUsers(ctx context.Context, userIDs []string) ([]*User, error) {
	const query = `
		SELECT
			id,
			first_name,
			last_name
		FROM
			auth_users
		WHERE
			id = ANY($1::text[]) AND is_active = true
	`

	var dbUsers []authUser
	err := a.dbConnectionPool.SelectContext(ctx, &dbUsers, query, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("error querying user IDs: %w", err)
	}
	if len(dbUsers) != len(userIDs) {
		return nil,
			fmt.Errorf(
				"error querying user IDs: searching for %d users, found %d users",
				len(userIDs),
				len(dbUsers),
			)
	}

	users := make([]*User, len(dbUsers))
	for i, u := range dbUsers {
		users[i] = &User{
			ID:        u.ID,
			FirstName: u.FirstName,
			LastName:  u.LastName,
		}
	}

	return users, nil
}

type defaultAuthenticatorOption func(a *defaultAuthenticator)

func newDefaultAuthenticator(options ...defaultAuthenticatorOption) *defaultAuthenticator {
	authenticator := &defaultAuthenticator{}

	for _, option := range options {
		option(authenticator)
	}

	return authenticator
}

func withAuthenticatorDatabaseConnectionPool(dbConnectionPool db.DBConnectionPool) defaultAuthenticatorOption {
	return func(a *defaultAuthenticator) {
		a.dbConnectionPool = dbConnectionPool
	}
}

func withPasswordEncrypter(passwordEncrypter PasswordEncrypter) defaultAuthenticatorOption {
	return func(a *defaultAuthenticator) {
		a.passwordEncrypter = passwordEncrypter
	}
}

func withResetTokenExpirationHours(expirationHours time.Duration) defaultAuthenticatorOption {
	return func(a *defaultAuthenticator) {
		a.resetTokenExpirationHours = expirationHours
	}
}

// Ensuring that defaultAuthenticator is implementing Authenticator interface
var _ Authenticator = (*defaultAuthenticator)(nil)
