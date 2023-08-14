package message

import (
	"fmt"
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
			wantErr:       fmt.Errorf("invalid message: email cannot be empty"),
		},
		{
			name:          "Email types need a valid email address",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{ToEmail: "invalid-email"},
			wantErr:       fmt.Errorf("invalid message: the provided email is not valid"),
		},
		{
			name:          "Email types need a title",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{ToEmail: "foo@test.com", Title: "   "},
			wantErr:       fmt.Errorf("title is empty"),
		},
		{
			name:          "[sms] message cannot be empty",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{ToEmail: "foo@test.com", Title: "My title"},
			wantErr:       fmt.Errorf("message is empty"),
		},
		{
			name:          "[email] all fields are present for AWS email 🎉",
			messengerType: MessengerTypeAWSEmail,
			message:       Message{ToEmail: "foo@test.com", Title: "My title", Message: "foo bar"},
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
			}
		})
	}
}
