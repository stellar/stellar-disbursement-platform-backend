package services

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
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
	messengerClientMock := message.MessengerClientMock{}
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	s := NewCreateUserService(models, dbConnectionPool, &authManagerMock, &messengerClientMock)

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

		u, err := s.CreateUser(ctx, newUser, "http://localhost:3000")
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

		u, err := s.CreateUser(ctx, newUser, "%invalid$%")
		assert.EqualError(t, err, `getting forgot password link: parse "%invalid$%": invalid URL escape "%in"`)
		assert.Nil(t, u)
	})

	t.Run("returns error when fails sending message", func(t *testing.T) {
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
		forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(htmltemplate.InvitationMessageTemplate{
			FirstName:          newUser.FirstName,
			Role:               newUser.Roles[0],
			ForgotPasswordLink: forgotPasswordLink,
			OrganizationName:   "MyCustomAid",
		})
		require.NoError(t, err)

		messengerClientMock.
			On("SendMessage", message.Message{
				ToEmail: newUser.Email,
				Title:   invitationMessageTitle,
				Message: content,
			}).
			Return(errors.New("unexpected error")).
			Once()

		u, err := s.CreateUser(ctx, newUser, uiBaseURL)
		assert.EqualError(t, err, `sending invitation email for user user-id: unexpected error`)
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
		forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(htmltemplate.InvitationMessageTemplate{
			FirstName:          newUser.FirstName,
			Role:               newUser.Roles[0],
			ForgotPasswordLink: forgotPasswordLink,
			OrganizationName:   "MyCustomAid",
		})
		require.NoError(t, err)

		messengerClientMock.
			On("SendMessage", message.Message{
				ToEmail: newUser.Email,
				Title:   invitationMessageTitle,
				Message: content,
			}).
			Return(nil).
			Once()

		u, err := s.CreateUser(ctx, newUser, uiBaseURL)
		require.NoError(t, err)

		assert.Equal(t, "user-id", u.ID)
		assert.Equal(t, newUser.FirstName, u.FirstName)
		assert.Equal(t, newUser.LastName, u.LastName)
		assert.Equal(t, newUser.Email, u.Email)
		assert.Equal(t, newUser.Roles, u.Roles)
		assert.Equal(t, newUser.IsOwner, u.IsOwner)
	})
}
