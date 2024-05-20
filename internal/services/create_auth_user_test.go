package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_CreateAuthUserService_CreateUser(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	authManagerMock := auth.AuthManagerMock{}
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	s := NewCreateUserService(models, dbConnectionPool, &authManagerMock)

	t.Run("returns error when can't create user", func(t *testing.T) {
		newUser := auth.User{
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			IsOwner:   true,
			Roles:     []string{"owner"},
		}

		authManagerMock.
			On("CreateUser", ctx, &newUser, "").
			Return(nil, errors.New("unexpected error")).
			Once()

		u, _, err := s.CreateUser(ctx, newUser, "http://localhost:3000")
		assert.EqualError(t, err, "creating new user: unexpected error")
		assert.Nil(t, u)
	})

	t.Run("returns error when can't get the forgot password link", func(t *testing.T) {
		newUser := auth.User{
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			IsOwner:   true,
			Roles:     []string{"owner"},
		}

		authManagerMock.
			On("CreateUser", ctx, &newUser, "").
			Return(&auth.User{}, nil).
			Once()

		u, _, err := s.CreateUser(ctx, newUser, "%invalid$%")
		assert.EqualError(t, err, `getting forgot password link: parse "%invalid$%": invalid URL escape "%in"`)
		assert.Nil(t, u)
	})

	t.Run("creates user and sends invitation message successfully", func(t *testing.T) {
		newUser := auth.User{
			FirstName: "First",
			LastName:  "Last",
			Email:     "email@email.com",
			IsOwner:   true,
			Roles:     []string{"owner"},
		}

		authManagerMock.
			On("CreateUser", ctx, &newUser, "").
			Return(&auth.User{
				ID:        "user-id",
				FirstName: newUser.FirstName,
				LastName:  newUser.LastName,
				Email:     newUser.Email,
				IsOwner:   true,
				Roles:     newUser.Roles,
			}, nil).
			Once()

		uiBaseURL := "http://localhost:3000"

		u, _, err := s.CreateUser(ctx, newUser, uiBaseURL)
		require.NoError(t, err)

		assert.Equal(t, "user-id", u.ID)
		assert.Equal(t, newUser.FirstName, u.FirstName)
		assert.Equal(t, newUser.LastName, u.LastName)
		assert.Equal(t, newUser.Email, u.Email)
		assert.Equal(t, newUser.Roles, u.Roles)
		assert.Equal(t, newUser.IsOwner, u.IsOwner)
	})
}
