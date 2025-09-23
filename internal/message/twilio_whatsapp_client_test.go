package message

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	twilioAPI "github.com/twilio/twilio-go/rest/api/v2010"
)

func Test_NewTwilioWhatsAppClient(t *testing.T) {
	testCases := []struct {
		name       string
		accountSid string
		authToken  string
		fromNumber string
		wantErr    error
	}{
		{
			name:    "accountSid cannot be empty",
			wantErr: fmt.Errorf("twilio WhatsApp accountSid is empty"),
		},
		{
			name:       "authToken cannot be empty",
			accountSid: "AC123456789",
			wantErr:    fmt.Errorf("twilio WhatsApp authToken is empty"),
		},
		{
			name:       "fromNumber cannot be empty",
			accountSid: "AC123456789",
			authToken:  "auth-token",
			wantErr:    fmt.Errorf("twilio WhatsApp fromNumber is empty"),
		},
		{
			name:       "fromNumber must be a valid phone number",
			accountSid: "AC123456789",
			authToken:  "auth-token",
			fromNumber: "invalid-phone",
			wantErr:    fmt.Errorf("twilio WhatsApp fromNumber is invalid: the provided phone number is not a valid E.164 number"),
		},
		{
			name:       "all fields are present with whatsapp: prefix",
			accountSid: "AC123456789",
			authToken:  "auth-token",
			fromNumber: "whatsapp:+14155238886",
		},
		{
			name:       "all fields are present without whatsapp: prefix",
			accountSid: "AC123456789",
			authToken:  "auth-token",
			fromNumber: "+14155238886",
		},
		{
			name:       "all fields are present with template SID",
			accountSid: "AC123456789",
			authToken:  "auth-token",
			fromNumber: "+14155238886",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewTwilioWhatsAppClient(tc.accountSid, tc.authToken, tc.fromNumber, map[MessageType]string{
				MessageTypeUserForgotPassword: "HX123",
				MessageTypeUserMFA:            "HX124",
				MessageTypeUserInvitation:     "HX125",
				MessageTypeReceiverInvitation: "HX126",
				MessageTypeReceiverOTP:        "HX127",
			})
			if tc.wantErr != nil {
				assert.Nil(t, client)
				assert.EqualError(t, err, tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, MessengerTypeTwilioWhatsApp, client.MessengerType())
			}
		})
	}
}

func Test_formatWhatsAppNumber(t *testing.T) {
	testCases := []struct {
		name        string
		phoneNumber string
		expected    string
	}{
		{
			name:        "adds whatsapp prefix",
			phoneNumber: "+14155238886",
			expected:    "whatsapp:+14155238886",
		},
		{
			name:        "keeps existing whatsapp prefix",
			phoneNumber: "whatsapp:+14155238886",
			expected:    "whatsapp:+14155238886",
		},
		{
			name:        "handles empty string",
			phoneNumber: "",
			expected:    "whatsapp:",
		},
		{
			name:        "handles whitespace",
			phoneNumber: "  +14155238886  ",
			expected:    "whatsapp:+14155238886",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatWhatsAppNumber(tc.phoneNumber)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_twilioWhatsAppClient_SendMessage_messageIsInvalid(t *testing.T) {
	client := &twilioWhatsAppClient{
		templates: map[MessageType]string{
			MessageTypeReceiverInvitation: "HX123",
		},
	}
	err := client.SendMessage(context.Background(), Message{})
	assert.EqualError(t, err, "validating WhatsApp message: invalid message: phone number cannot be empty")
}

func Test_twilioWhatsAppClient_SendMessage_errorIsHandledCorrectly(t *testing.T) {
	ctx := context.Background()
	message := Message{
		Type:          MessageTypeReceiverInvitation,
		ToPhoneNumber: "+14155551234",
		Body:          "Test WhatsApp message",
	}

	mockAPI := newMockTwilioAPIInterface(t)
	expectedError := fmt.Errorf("test Twilio API error")

	mockAPI.On("CreateMessage", mock.MatchedBy(func(params *twilioAPI.CreateMessageParams) bool {
		return params.To != nil && *params.To == "whatsapp:+14155551234" &&
			params.From != nil && *params.From == "whatsapp:+14155238886" &&
			params.ContentSid != nil && *params.ContentSid == "HX123"
	})).Return(nil, expectedError).Once()

	client := &twilioWhatsAppClient{
		apiService: mockAPI,
		fromNumber: "+14155238886",
		templates: map[MessageType]string{
			MessageTypeReceiverInvitation: "HX123",
		},
	}

	err := client.SendMessage(ctx, message)
	assert.EqualError(t, err, "sending Twilio WhatsApp message: test Twilio API error")
}

func Test_twilioWhatsAppClient_SendMessage_handlesAPIError(t *testing.T) {
	ctx := context.Background()
	message := Message{
		Type:          MessageTypeReceiverInvitation,
		ToPhoneNumber: "+14155551234",
		Body:          "Test WhatsApp message",
	}

	mockAPI := newMockTwilioAPIInterface(t)
	errorCode := 21211
	errorMessage := "Invalid 'To' Phone Number"

	mockAPI.On("CreateMessage", mock.MatchedBy(func(params *twilioAPI.CreateMessageParams) bool {
		return params.To != nil && *params.To == "whatsapp:+14155551234"
	})).Return(&twilioAPI.ApiV2010Message{
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
	}, nil).Once()

	client := &twilioWhatsAppClient{
		apiService: mockAPI,
		fromNumber: "+14155238886",
		templates: map[MessageType]string{
			MessageTypeReceiverInvitation: "HX123",
		},
	}

	err := client.SendMessage(ctx, message)
	assert.EqualError(t, err, "sending Twilio message returned an error {code= \"21211\", message= \"Invalid 'To' Phone Number\"}")
}

func Test_twilioWhatsAppClient_SendMessage_success(t *testing.T) {
	ctx := context.Background()
	message := Message{
		Type:          MessageTypeReceiverInvitation,
		ToPhoneNumber: "+14155551234",
		Body:          "Test WhatsApp message",
	}

	mockAPI := newMockTwilioAPIInterface(t)

	mockAPI.On("CreateMessage", mock.MatchedBy(func(params *twilioAPI.CreateMessageParams) bool {
		return params.To != nil && *params.To == "whatsapp:+14155551234" &&
			params.From != nil && *params.From == "whatsapp:+14155238886" &&
			params.ContentSid != nil && *params.ContentSid == "HX123"
	})).Return(&twilioAPI.ApiV2010Message{
		ErrorCode:    nil,
		ErrorMessage: nil,
	}, nil).Once()

	client := &twilioWhatsAppClient{
		apiService: mockAPI,
		fromNumber: "+14155238886",
		templates: map[MessageType]string{
			MessageTypeReceiverInvitation: "HX123",
		},
	}

	err := client.SendMessage(ctx, message)
	assert.NoError(t, err)
}

func Test_twilioWhatsAppClient_SendMessage_withWhatsAppPrefixedFromNumber(t *testing.T) {
	ctx := context.Background()
	message := Message{
		Type:          MessageTypeReceiverInvitation,
		ToPhoneNumber: "+14155551234",
		Body:          "Test WhatsApp message",
	}

	mockAPI := newMockTwilioAPIInterface(t)

	mockAPI.On("CreateMessage", mock.MatchedBy(func(params *twilioAPI.CreateMessageParams) bool {
		return params.To != nil && *params.To == "whatsapp:+14155551234" &&
			params.From != nil && *params.From == "whatsapp:+14155238886" &&
			params.ContentSid != nil && *params.ContentSid == "HX123"
	})).Return(&twilioAPI.ApiV2010Message{
		ErrorCode:    nil,
		ErrorMessage: nil,
	}, nil).Once()

	client := &twilioWhatsAppClient{
		apiService: mockAPI,
		fromNumber: "whatsapp:+14155238886", // Already has whatsapp: prefix
		templates: map[MessageType]string{
			MessageTypeReceiverInvitation: "HX123",
		},
	}

	err := client.SendMessage(ctx, message)
	assert.NoError(t, err)
}

func Test_twilioWhatsAppClient_MessengerType(t *testing.T) {
	client := &twilioWhatsAppClient{}
	assert.Equal(t, MessengerTypeTwilioWhatsApp, client.MessengerType())
}

func Test_twilioWhatsAppClient_SendMessage_withTemplate(t *testing.T) {
	ctx := context.Background()
	message := Message{
		Type:          MessageTypeReceiverInvitation,
		ToPhoneNumber: "+14155551234",
		TemplateVariables: map[string]string{
			"1": "Test Organization",
			"2": "https://example.com/register?token=abc123",
		},
	}

	mockAPI := newMockTwilioAPIInterface(t)

	mockAPI.On("CreateMessage", mock.MatchedBy(func(params *twilioAPI.CreateMessageParams) bool {
		return params.To != nil && *params.To == "whatsapp:+14155551234" &&
			params.From != nil && *params.From == "whatsapp:+14155238886" &&
			params.ContentSid != nil && *params.ContentSid == "HXabcdef123456789" &&
			params.ContentVariables != nil && *params.ContentVariables == `{"1":"Test Organization","2":"https://example.com/register?token=abc123"}` &&
			params.Body == nil // Should not have Body when using template
	})).Return(&twilioAPI.ApiV2010Message{
		ErrorCode:    nil,
		ErrorMessage: nil,
	}, nil).Once()

	client := &twilioWhatsAppClient{
		apiService: mockAPI,
		fromNumber: "+14155238886",
		templates: map[MessageType]string{
			MessageTypeReceiverInvitation: "HXabcdef123456789",
		},
	}

	err := client.SendMessage(ctx, message)
	assert.NoError(t, err)
}

func Test_twilioWhatsAppClient_SendMessage_withDefaultTemplate(t *testing.T) {
	ctx := context.Background()
	message := Message{
		Type:          MessageTypeReceiverInvitation,
		ToPhoneNumber: "+14155551234",
		Body:          "Hello from Test Organization! Click here to register: https://example.com/register?token=abc123",
	}

	mockAPI := newMockTwilioAPIInterface(t)

	mockAPI.On("CreateMessage", mock.MatchedBy(func(params *twilioAPI.CreateMessageParams) bool {
		return params.To != nil && *params.To == "whatsapp:+14155551234" &&
			params.From != nil && *params.From == "whatsapp:+14155238886" &&
			params.ContentSid != nil && *params.ContentSid == "HXdefault123456" &&
			params.Body == nil // Should not have Body when using template
	})).Return(&twilioAPI.ApiV2010Message{
		ErrorCode:    nil,
		ErrorMessage: nil,
	}, nil).Once()

	client := &twilioWhatsAppClient{
		apiService: mockAPI,
		fromNumber: "+14155238886",
		templates: map[MessageType]string{
			MessageTypeReceiverInvitation: "HXdefault123456",
		},
	}

	err := client.SendMessage(ctx, message)
	assert.NoError(t, err)
}

func Test_Message_ValidateFor_WhatsAppTemplate(t *testing.T) {
	testCases := []struct {
		name    string
		message Message
		wantErr bool
	}{
		{
			name: "valid template message without body",
			message: Message{
				Type:          MessageTypeReceiverInvitation,
				ToPhoneNumber: "+14155551234",
			},
			wantErr: false,
		},
		{
			name: "valid template message with body and variables",
			message: Message{
				Type:              MessageTypeReceiverInvitation,
				ToPhoneNumber:     "+14155551234",
				Body:              "Some content",
				TemplateVariables: map[string]string{"1": "Test"},
			},
			wantErr: false,
		},
		{
			name: "invalid phone number",
			message: Message{
				Type:          MessageTypeReceiverInvitation,
				ToPhoneNumber: "invalid",
			},
			wantErr: true,
		},
		{
			name: "missing phone and template - should fail",
			message: Message{
				Body: "Some message",
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.message.ValidateFor(MessengerTypeTwilioWhatsApp)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
