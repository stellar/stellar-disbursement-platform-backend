package message

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_message_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		messengerType MessengerType
		message       Message
		wantErr       error
	}{
		// SMS types
		{
			name:          "SMS types need a non-empty phone number",
			messengerType: MessengerTypeTwilioSMS,
			message:       Message{},
			wantErr:       fmt.Errorf("invalid message: phone number cannot be empty"),
		},
		{
			name:          "SMS types need a valid phone number",
			messengerType: MessengerTypeTwilioSMS,
			message:       Message{ToPhoneNumber: "invalid-phone"},
			wantErr:       fmt.Errorf("invalid message: the provided phone number is not a valid E.164 number"),
		},
		{
			name:          "[sms] message cannot be empty",
			messengerType: MessengerTypeTwilioSMS,
			message:       Message{ToPhoneNumber: "+14152111111", Message: "   "},
			wantErr:       fmt.Errorf("message is empty"),
		},
		{
			name:          "[sms] all fields are present for Twilio 🎉",
			messengerType: MessengerTypeTwilioSMS,
			message:       Message{ToPhoneNumber: "+14152111111", Message: "foo bar"},
			wantErr:       nil,
		},
		{
			name:          "[sms] all fields are present for AWS SNS 🎉",
			messengerType: MessengerTypeAWSSMS,
			message:       Message{ToPhoneNumber: "+14152111111", Message: "foo bar"},
			wantErr:       nil,
		},
		// Email types
		{
			name:          "Email types need a non-empty email address",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{},
			wantErr:       fmt.Errorf("invalid e-mail: invalid email format: email cannot be empty"),
		},
		{
			name:          "Email types need a valid email address",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{ToEmail: "invalid-email"},
			wantErr:       fmt.Errorf("invalid e-mail: invalid email format: the provided email %q is not valid", "invalid-email"),
		},
		{
			name:          "Email types need a title",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{ToEmail: "FOO@test.com", Title: "   "},
			wantErr:       fmt.Errorf("invalid e-mail: title is empty"),
		},
		{
			name:          "[sms] message cannot be empty",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{ToEmail: "FOO@test.com", Title: "My title"},
			wantErr:       fmt.Errorf("message is empty"),
		},
		{
			name:          "[email] all fields are present for AWS email 🎉",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{ToEmail: "FOO@test.com", Title: "My title", Message: "foo bar"},
			wantErr:       nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.message.ValidateFor(tc.messengerType)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
			} else {
				require.NoError(t, err)
				if !slices.Contains([]string{"", "invalid-email"}, tc.message.ToEmail) {
					require.Equal(t, "foo@test.com", tc.message.ToEmail)
				}
			}
		})
	}
}

func TestMessage_SupportedChannels(t *testing.T) {
	testCases := []struct {
		name         string
		message      Message
		wantChannels []MessageChannel
	}{
		{
			name:         "sms only",
			message:      Message{ToPhoneNumber: "+14152111111", Message: "Hello"},
			wantChannels: []MessageChannel{MessageChannelSMS},
		},
		{
			name:         "e-mail only",
			message:      Message{ToEmail: "test@example.com", Title: "Test", Message: "Hello"},
			wantChannels: []MessageChannel{MessageChannelEmail},
		},
		{
			name:         "both sms and e-mail",
			message:      Message{ToPhoneNumber: "+14152111111", ToEmail: "test@example.com", Title: "Test", Message: "Hello"},
			wantChannels: []MessageChannel{MessageChannelSMS, MessageChannelEmail},
		},
		{
			name:         "neither sms nor e-mail",
			message:      Message{Message: "Hello"},
			wantChannels: []MessageChannel{},
		},
		{
			name:         "invalid phone number",
			message:      Message{ToPhoneNumber: "invalid", ToEmail: "test@example.com", Title: "Test", Message: "Hello"},
			wantChannels: []MessageChannel{MessageChannelEmail},
		},
		{
			name:         "invalid email",
			message:      Message{ToPhoneNumber: "+14152111111", ToEmail: "invalid", Title: "Test", Message: "Hello"},
			wantChannels: []MessageChannel{MessageChannelSMS},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotChannels := tc.message.SupportedChannels()
			require.ElementsMatch(t, tc.wantChannels, gotChannels)
		})
	}
}

func TestMessage_String(t *testing.T) {
	testCases := []struct {
		name               string
		message            Message
		wantRepresentation string
	}{
		{
			name:               "all fields present",
			message:            Message{ToPhoneNumber: "+14152111111", ToEmail: "test@example.com", Title: "Test Title", Message: "Hello, World!"},
			wantRepresentation: "Message{ToPhoneNumber: +14...111, ToEmail: tes...com, Message: Hel...ld!, Title: Tes...tle}",
		},
		{
			name:               "only phone number",
			message:            Message{ToPhoneNumber: "+14152111111", Message: "Hello"},
			wantRepresentation: "Message{ToPhoneNumber: +14...111, ToEmail: , Message: Hello, Title: }",
		},
		{
			name:               "only email",
			message:            Message{ToEmail: "test@example.com", Title: "Test", Message: "Hello"},
			wantRepresentation: "Message{ToPhoneNumber: , ToEmail: tes...com, Message: Hello, Title: Test}",
		},
		{
			name:               "empty message",
			message:            Message{},
			wantRepresentation: "Message{ToPhoneNumber: , ToEmail: , Message: , Title: }",
		},
		{
			name:               "long fields",
			message:            Message{ToPhoneNumber: "+14152111111", ToEmail: "very.long.email@example.com", Title: "This is a very long title", Message: "This is a very long message that should be truncated"},
			wantRepresentation: "Message{ToPhoneNumber: +14...111, ToEmail: ver...com, Message: Thi...ted, Title: Thi...tle}",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotRepresentation := tc.message.String()
			require.Equal(t, tc.wantRepresentation, gotRepresentation)
		})
	}
}
