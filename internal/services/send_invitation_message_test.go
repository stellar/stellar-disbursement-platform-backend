package services

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

func TestSendInvitationMessageOptions_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		options SendInvitationMessageOptions
		errStr  string
	}{
		{
			name:    "missing first name",
			options: SendInvitationMessageOptions{},
			errStr:  "first name is required",
		},
		{
			name:    "missing email",
			options: SendInvitationMessageOptions{FirstName: "foobar"},
			errStr:  "email is required",
		},
		{
			name:    "missing role",
			options: SendInvitationMessageOptions{FirstName: "foobar", Email: "foo@bar.com"},
			errStr:  "role is required",
		},
		{
			name:    "missing ui base URL",
			options: SendInvitationMessageOptions{FirstName: "foobar", Email: "foo@bar.com", Role: "owner"},
			errStr:  "UI base URL is required",
		},
		{
			name:    "invalid ui base URL",
			options: SendInvitationMessageOptions{FirstName: "foobar", Email: "foo@bar.com", Role: "owner", UIBaseURL: "%invalid$%"},
			errStr:  `UI base URL is not a valid URL: parse "%invalid$%": invalid URL escape "%in"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.options.Validate()
			if tc.errStr != "" {
				assert.EqualError(t, err, tc.errStr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_SendInvitationMessage(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)

	ctx := context.Background()
	firstName := "First"
	email := "email@email.com"
	roles := []string{"owner"}
	uiBaseURL := "http://localhost:3000"

	forgotPasswordLink, err := url.JoinPath(uiBaseURL, "forgot-password")
	require.NoError(t, err)

	content, err := htmltemplate.ExecuteHTMLTemplateForStaffInvitationEmailMessage(htmltemplate.StaffInvitationEmailMessageTemplate{
		FirstName:          firstName,
		Role:               roles[0],
		ForgotPasswordLink: forgotPasswordLink,
		OrganizationName:   "MyCustomAid",
	})
	require.NoError(t, err)

	invitationMessageTitle := "Welcome to Stellar Disbursement Platform"

	testCases := []struct {
		name        string
		options     SendInvitationMessageOptions
		mockSetup   func(t *testing.T, msgClientMock *message.MessengerClientMock)
		expectedErr string
	}{
		{
			name: "returns error when options are not valid",
			options: SendInvitationMessageOptions{
				FirstName: firstName,
				Email:     email,
				Role:      roles[0],
				UIBaseURL: "%invalid$%",
			},
			mockSetup: func(t *testing.T, msgClientMock *message.MessengerClientMock) {
				msgClientMock.
					AssertNotCalled(t, "SendMessage")
			},
			expectedErr: "invalid options: UI base URL is not a valid URL: parse \"%invalid$%\": invalid URL escape \"%in\"",
		},
		{
			name: "returns error when failing to send invitation message",
			options: SendInvitationMessageOptions{
				FirstName: firstName,
				Email:     email,
				Role:      roles[0],
				UIBaseURL: uiBaseURL,
			},
			mockSetup: func(t *testing.T, msgClientMock *message.MessengerClientMock) {
				msgClientMock.
					On("SendMessage", mock.Anything, message.Message{
						Type:    message.MessageTypeUserInvitation,
						ToEmail: email,
						Title:   invitationMessageTitle,
						Body:    content,
						TemplateVariables: map[message.TemplateVariable]string{
							message.TemplateVarFirstName:          firstName,
							message.TemplateVarRole:               roles[0],
							message.TemplateVarForgotPasswordLink: forgotPasswordLink,
							message.TemplateVarOrgName:            "MyCustomAid",
						},
					}).
					Return(errors.New("foobar")).
					Once()
			},
			expectedErr: "sending invitation message via messenger client: foobar",
		},
		{
			name: "sends invitation message successfully",
			options: SendInvitationMessageOptions{
				FirstName: firstName,
				Email:     email,
				Role:      roles[0],
				UIBaseURL: uiBaseURL,
			},
			mockSetup: func(t *testing.T, msgClientMock *message.MessengerClientMock) {
				msgClientMock.
					On("SendMessage", mock.Anything, message.Message{
						Type:    message.MessageTypeUserInvitation,
						ToEmail: email,
						Title:   invitationMessageTitle,
						Body:    content,
						TemplateVariables: map[message.TemplateVariable]string{
							message.TemplateVarFirstName:          firstName,
							message.TemplateVarRole:               roles[0],
							message.TemplateVarForgotPasswordLink: forgotPasswordLink,
							message.TemplateVarOrgName:            "MyCustomAid",
						},
					}).
					Return(nil).
					Once()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			messengerClientMock := message.MessengerClientMock{}

			tc.mockSetup(t, &messengerClientMock)

			err := SendInvitationMessage(ctx, &messengerClientMock, models, tc.options)
			if tc.expectedErr != "" {
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}

			messengerClientMock.AssertExpectations(t)
		})
	}
}
