package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/utils"
	"github.com/stretchr/testify/require"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

type RandomAuthUser struct {
	ID                string
	Email             string
	FirstName         string
	LastName          string
	Password          string
	EncryptedPassword string
	IsOwner           bool
	IsActive          bool
	Roles             []string
	CreatedAt         time.Time
}

func (rau *RandomAuthUser) ToUser() *User {
	return &User{
		ID:        rau.ID,
		FirstName: rau.FirstName,
		LastName:  rau.LastName,
		Email:     rau.Email,
		IsOwner:   rau.IsOwner,
		IsActive:  rau.IsActive,
		Roles:     rau.Roles,
	}
}

func randStringRunes(t *testing.T, n int) string {
	b := make([]rune, n)
	for i := range b {
		randomNumber, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		require.NoError(t, err)

		b[i] = letterRunes[randomNumber.Int64()]
	}
	return string(b)
}

func CreateRandomAuthUserFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, passwordEncrypter PasswordEncrypter, isAdmin bool, roles ...string) *RandomAuthUser {
	randomSuffix := randStringRunes(t, 5)
	email := fmt.Sprintf("email%s@randomemail.com", randomSuffix)
	password := "password" + randomSuffix
	firstName := "firstName" + randomSuffix
	lastName := "lastName" + randomSuffix

	encryptedPassword, err := passwordEncrypter.Encrypt(ctx, password)
	require.NoError(t, err)

	const query = `
		INSERT INTO auth_users
			(email, encrypted_password, is_owner, roles, first_name, last_name)
		VALUES
			($1, $2, $3, $4, $5, $6)
		RETURNING
			id, created_at
	`

	user := &RandomAuthUser{
		Email:             email,
		FirstName:         firstName,
		LastName:          lastName,
		Password:          password,
		IsOwner:           isAdmin,
		IsActive:          true,
		EncryptedPassword: encryptedPassword,
		Roles:             roles,
	}
	err = sqlExec.QueryRowxContext(ctx, query, email, encryptedPassword, isAdmin, pq.Array(roles), firstName, lastName).Scan(&user.ID, &user.CreatedAt)
	require.NoError(t, err)

	return user
}

func CreateResetPasswordTokenFixture(t *testing.T, ctx context.Context, sqlExec db.SQLExecuter, randomAuthUser *RandomAuthUser, isValid bool, createdAt time.Time) (token string) {
	resetToken, err := utils.StringWithCharset(resetTokenLength, utils.DefaultCharset)
	require.NoError(t, err)

	q := `
		INSERT INTO
			auth_user_password_reset (token, auth_user_id, is_valid, created_at)
		VALUES
			($1, $2, $3, $4)
	`
	_, err = sqlExec.ExecContext(ctx, q, resetToken, randomAuthUser.ID, isValid, createdAt)
	require.NoError(t, err)

	return resetToken
}
