package message

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseMessengerType(t *testing.T) {
	testCases := []struct {
		messengerType string
		wantErr       error
	}{
		{wantErr: fmt.Errorf("invalid message sender type \"\"")},
		{messengerType: "foo_BAR", wantErr: fmt.Errorf("invalid message sender type \"FOO_BAR\"")},
		{messengerType: "TWILIO_SMS"},
		{messengerType: "TWILIO_WHATSAPP"},
		{messengerType: "TWILIO_EMAIL"},
		{messengerType: "tWiLiO_SMS"},
		{messengerType: "AWS_SMS"},
		{messengerType: "AWS_EMAIL"},
		{messengerType: "DRY_RUN"},
	}

	for _, tc := range testCases {
		t.Run("messengerType: "+tc.messengerType, func(t *testing.T) {
			_, err := ParseMessengerType(tc.messengerType)
			if tc.wantErr != nil {
				assert.Equal(t, tc.wantErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_GetClient(t *testing.T) {
	// MessengerTypeTwilioSMS
	messengerType := MessengerTypeTwilioSMS
	opts := MessengerOptions{
		MessengerType:    messengerType,
		TwilioAccountSID: "accountSid",
		TwilioAuthToken:  "authToken",
		TwilioServiceSID: "senderID",
	}
	gotClient, err := GetClient(opts)
	require.NoError(t, err)
	require.IsType(t, &twilioClient{}, gotClient)

	// MessengerTypeAWSSMS
	messengerType = MessengerTypeAWSSMS
	opts = MessengerOptions{
		MessengerType:      messengerType,
		AWSAccessKeyID:     "accessKeyID",
		AWSSecretAccessKey: "secretAccessKey",
		AWSRegion:          "region",
		AWSSNSSenderID:     "mySenderID",
	}
	gotClient, err = GetClient(opts)
	require.NoError(t, err)
	require.IsType(t, &awsSNSClient{}, gotClient)
	gotAWSSNSClient, ok := gotClient.(*awsSNSClient)
	require.True(t, ok)
	require.NotNil(t, gotAWSSNSClient.snsService)

	// MessengerTypeTwilioWhatsApp
	messengerType = MessengerTypeTwilioWhatsApp
	opts = MessengerOptions{
		MessengerType:            messengerType,
		TwilioAccountSID:         "AC123456789",
		TwilioAuthToken:          "auth-token",
		TwilioWhatsAppFromNumber: "+14155238886",
		TwilioWhatsAppReceiverInvitationTemplateSID: "HXabcdef123456784",
		TwilioWhatsAppReceiverOTPTemplateSID:        "HXabcdef123456783",
		TwilioWhatsAppUserInvitationTemplateSID:     "HXabcdef123456782",
		TwilioWhatsAppUserForgotPasswordTemplateSID: "HXabcdef123456781",
		TwilioWhatsAppUserMFATemplateSID:            "HXabcdef123456780",
	}
	gotClient, err = GetClient(opts)
	require.NoError(t, err)
	assert.IsType(t, &twilioWhatsAppClient{}, gotClient)

	// MessengerTypeAWSEmail
	messengerType = MessengerTypeAWSEmail
	opts = MessengerOptions{
		MessengerType:      messengerType,
		AWSAccessKeyID:     "accessKeyID",
		AWSSecretAccessKey: "secretAccessKey",
		AWSRegion:          "region",
		AWSSESSenderID:     "foo@test.com",
	}
	gotClient, err = GetClient(opts)
	require.NoError(t, err)
	require.IsType(t, &awsSESClient{}, gotClient)
	gotAWSSESClient, ok := gotClient.(*awsSESClient)
	require.True(t, ok)
	require.NotNil(t, gotAWSSESClient.emailService)
}
