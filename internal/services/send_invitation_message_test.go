package services

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

func Test_SendInvitationMessage(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	messengerClientMock := message.MessengerClientMock{}
	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	firstName := "First"
	email := "email@email.com"
	roles := []string{"owner"}

	t.Run("returns error when can't get the forgot password link", func(t *testing.T) {
		err := SendInvitationMessage(ctx, &messengerClientMock, models, firstName, roles[0], email, "%invalid$%")
		assert.EqualError(t, err, `getting forgot password link: parse "%invalid$%": invalid URL escape "%in"`)
	})

	t.Run("sends invitation message successfully", func(t *testing.T) {
		uiBaseURL := "http://localhost:3000"
		forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
		require.NoError(t, err)

		content, err := htmltemplate.ExecuteHTMLTemplateForInvitationMessage(htmltemplate.InvitationMessageTemplate{
			FirstName:          firstName,
			Role:               roles[0],
			ForgotPasswordLink: forgotPasswordLink,
			OrganizationName:   "MyCustomAid",
		})
		require.NoError(t, err)

		messengerClientMock.
			On("SendMessage", message.Message{
				ToEmail: email,
				Title:   invitationMessageTitle,
				Message: content,
			}).
			Return(nil).
			Once()

		err = SendInvitationMessage(ctx, &messengerClientMock, models, firstName, roles[0], email, uiBaseURL)
		require.NoError(t, err)
	})
}
