package message

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sendgrid/rest"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func (m *mockTwilioSendGridClient) Send(email *mail.SGMailV3) (*rest.Response, error) {
	args := m.Called(email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*rest.Response), args.Error(1)
}

func Test_NewTwilioSendGridClient(t *testing.T) {
	testCases := []struct {
		name          string
		apiKey        string
		senderAddress string
		wantErr       error
	}{
		{
			name:    "apiKey cannot be empty",
			wantErr: fmt.Errorf("sendGrid API key is empty"),
		},
		{
			name:          "senderAddress needs to be a valid email",
			apiKey:        "api-key",
			senderAddress: "invalid-email",
			wantErr:       fmt.Errorf("sendGrid senderAddress is invalid: the email address provided is not valid"),
		},
		{
			name:          "all fields are present",
			apiKey:        "api-key",
			senderAddress: "foo@stellar.org",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewTwilioSendGridClient(tc.apiKey, tc.senderAddress)
			if tc.wantErr != nil {
				assert.EqualError(t, err, tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_TwilioSendGridClient_SendMessage_messageIsInvalid(t *testing.T) {
	var mSendGrid MessengerClient = &twilioSendGridClient{}
	err := mSendGrid.SendMessage(Message{})
	assert.EqualError(t, err, "validating message to send an email through SendGrid: invalid e-mail: invalid email format: email field is required")
}

func Test_TwilioSendGridClient_SendMessage_errorIsHandledCorrectly(t *testing.T) {
	message := Message{ToEmail: "foo@stellar.org", Title: "test title", Body: "foo bar"}

	mSendGrid := newMockTwilioSendGridClient(t)

	// MatchBy is used to match the email that is being sent
	mSendGrid.On("Send", mock.MatchedBy(func(email *mail.SGMailV3) bool {
		// Verify the email content matches what we expect
		return email.From.Address == "sender@stellar.org" &&
			email.Subject == message.Title &&
			len(email.Personalizations) == 1 &&
			len(email.Personalizations[0].To) == 1 &&
			email.Personalizations[0].To[0].Address == message.ToEmail
	})).Return(nil, fmt.Errorf("test SendGrid error")).Once()

	client := &twilioSendGridClient{
		client:        mSendGrid,
		senderAddress: "sender@stellar.org",
	}

	err := client.SendMessage(message)
	assert.EqualError(t, err, "sending SendGrid email: test SendGrid error")
}

func Test_TwilioSendGridClient_SendMessage_handlesAPIError(t *testing.T) {
	message := Message{ToEmail: "foo@stellar.org", Title: "test title", Body: "foo bar"}

	mSendGrid := newMockTwilioSendGridClient(t)

	mSendGrid.On("Send", mock.MatchedBy(func(email *mail.SGMailV3) bool {
		return email.From.Address == "sender@stellar.org" &&
			email.Subject == message.Title
	})).Return(&rest.Response{
		StatusCode: 400,
		Body:       "Bad Request",
	}, nil).Once()

	client := &twilioSendGridClient{
		client:        mSendGrid,
		senderAddress: "sender@stellar.org",
	}

	err := client.SendMessage(message)
	assert.EqualError(t, err, "sendGrid API returned error status code= 400, body= Bad Request")
}

func Test_TwilioSendGrid_SendMessage_success(t *testing.T) {
	message := Message{ToEmail: "foo@stellar.org", Title: "test title", Body: "foo bar"}

	mSendGrid := newMockTwilioSendGridClient(t)

	successResponse := &rest.Response{
		StatusCode: 202,
		Body:       "Accepted",
	}

	mSendGrid.On("Send", mock.MatchedBy(func(email *mail.SGMailV3) bool {
		// Verify plain text was converted to HTML
		expectedHTML := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta http-equiv="X-UA-Compatible" content="IE=edge,chrome=1">
</head>
<body>
foo bar
</body>
</html>
`
		expectedHTML = strings.TrimSpace(expectedHTML)

		gotContent := email.Content[0].Value
		gotContent = strings.TrimSpace(gotContent)

		return email.From.Address == "sender@stellar.org" &&
			email.Subject == message.Title &&
			len(email.Personalizations) == 1 &&
			len(email.Personalizations[0].To) == 1 &&
			email.Personalizations[0].To[0].Address == message.ToEmail &&
			gotContent == expectedHTML
	})).Return(successResponse, nil).Once()

	client := &twilioSendGridClient{
		client:        mSendGrid,
		senderAddress: "sender@stellar.org",
	}

	err := client.SendMessage(message)
	assert.NoError(t, err)
}

func Test_TwilioSendGrid_SendMessage_withHTMLContent(t *testing.T) {
	htmlContent := "<html><body><h1>Hello</h1></body></html>"
	message := Message{ToEmail: "foo@stellar.org", Title: "test title", Body: htmlContent}

	mSendGrid := newMockTwilioSendGridClient(t)

	successResponse := &rest.Response{
		StatusCode: 202,
		Body:       "Accepted",
	}

	mSendGrid.On("Send", mock.MatchedBy(func(email *mail.SGMailV3) bool {
		gotContent := email.Content[0].Value
		gotContent = strings.TrimSpace(gotContent)

		return email.From.Address == "sender@stellar.org" &&
			email.Subject == message.Title &&
			gotContent == htmlContent // Should use original HTML content
	})).Return(successResponse, nil).Once()

	client := &twilioSendGridClient{
		client:        mSendGrid,
		senderAddress: "sender@stellar.org",
	}

	err := client.SendMessage(message)
	assert.NoError(t, err)
}

// mockTwilioSendGridClient implements twilioSendGridInterface for testing
type mockTwilioSendGridClient struct {
	mock.Mock
}

type testInterface interface {
	mock.TestingT
	Cleanup(func())
}

func newMockTwilioSendGridClient(t testInterface) *mockTwilioSendGridClient {
	mock := &mockTwilioSendGridClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
