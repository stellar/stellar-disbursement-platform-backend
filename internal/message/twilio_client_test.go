package message

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twilio/twilio-go"
	twilioAPI "github.com/twilio/twilio-go/rest/api/v2010"
)

func Test_NewTwilioClient(t *testing.T) {
	// Declare types in advance to make sure these are the types being returned
	var gotTwilioClient MessengerClient
	var err error

	// accountSid cannot be empty
	gotTwilioClient, err = NewTwilioClient("", "", "")
	require.Nil(t, gotTwilioClient)
	require.EqualError(t, err, "twilio accountSid is empty")

	// accountSid cannot be empty
	gotTwilioClient, err = NewTwilioClient("accountSid", "  ", "")
	require.Nil(t, gotTwilioClient)
	require.EqualError(t, err, "twilio authToken is empty")

	// senderID cannot be empty
	gotTwilioClient, err = NewTwilioClient("accountSid", "authToken", "")
	require.Nil(t, gotTwilioClient)
	require.EqualError(t, err, "twilio senderID is empty")

	// all fields are present 🎉
	gotTwilioClient, err = NewTwilioClient("accountSid", "authToken", "senderID")
	require.NoError(t, err)
	wantTwilioClient := &twilioClient{
		apiService: twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: "accountSid",
			Password: "authToken",
		}).Api,
		senderID: "senderID",
	}
	require.Equal(t, wantTwilioClient, gotTwilioClient)
}

func Test_Twilio_messengerType(t *testing.T) {
	tw := twilioClient{}
	require.Equal(t, MessengerTypeTwilioSMS, tw.MessengerType())
}

func Test_Twilio_SendMessage_messageIsInvalid(t *testing.T) {
	var mTwilio MessengerClient = &twilioClient{}
	err := mTwilio.SendMessage(context.Background(), Message{})
	require.EqualError(t, err, "validating SMS message: invalid message: phone number cannot be empty")
}

func Test_Twilio_SendMessage_errorIsHandledCorrectly(t *testing.T) {
	// check if error is handled correctly
	testPhoneNumber := "+14155111111"
	testMessage := "foo bar"
	testSenderID := "senderID"
	mTwilioApi := newMockTwilioApiInterface(t)
	mTwilioApi.
		On("CreateMessage", &twilioAPI.CreateMessageParams{
			To:                  &testPhoneNumber,
			Body:                &testMessage,
			MessagingServiceSid: &testSenderID,
		}).
		Return(nil, fmt.Errorf("test twilio error")).
		Once()

	mTwilio := twilioClient{apiService: mTwilioApi, senderID: "senderID"}
	err := mTwilio.SendMessage(context.Background(), Message{ToPhoneNumber: "+14155111111", Body: "foo bar"})
	assert.EqualError(t, err, "sending Twilio SMS: test twilio error")
}

func Test_Twilio_SendMessage_doesntReturnErrorButResponseContainsErrorEmbedded(t *testing.T) {
	// validate the case where the response contains an error message,
	// despite the method succeeding
	testPhoneNumber2 := "+14152222222"
	testMessage2 := "foo bar"
	testSenderID := "senderID"

	wantErrCode := 12345
	wantErrMessage := "Foo bar error message"

	mTwilioApi := newMockTwilioApiInterface(t)
	mTwilioApi.
		On("CreateMessage", &twilioAPI.CreateMessageParams{
			To:                  &testPhoneNumber2,
			Body:                &testMessage2,
			MessagingServiceSid: &testSenderID,
		}).
		Return(&twilioAPI.ApiV2010Message{
			ErrorCode:    &wantErrCode,
			ErrorMessage: &wantErrMessage,
		}, nil).
		Once()

	mTwilio := twilioClient{apiService: mTwilioApi, senderID: "senderID"}
	err := mTwilio.SendMessage(context.Background(), Message{ToPhoneNumber: "+14152222222", Body: "foo bar"})
	assert.EqualError(t, err, `sending Twilio message returned an error {code= "12345", message= "Foo bar error message"}`)
}

func Test_Twilio_SendMessage_success(t *testing.T) {
	// check if error is handled correctly
	testPhoneNumber := "+14153333333"
	testMessage := "foo bar"
	testSenderID := "senderID"
	mTwilioApi := newMockTwilioApiInterface(t)
	mTwilioApi.
		On("CreateMessage", &twilioAPI.CreateMessageParams{
			To:                  &testPhoneNumber,
			Body:                &testMessage,
			MessagingServiceSid: &testSenderID,
		}).
		Return(&twilioAPI.ApiV2010Message{}, nil).
		Once()

	mTwilio := twilioClient{apiService: mTwilioApi, senderID: "senderID"}
	err := mTwilio.SendMessage(context.Background(), Message{ToPhoneNumber: "+14153333333", Body: "foo bar"})
	require.NoError(t, err)
}
