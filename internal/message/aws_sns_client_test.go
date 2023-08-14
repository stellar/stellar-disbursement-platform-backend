package message

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockAWSSNSClient struct {
	mock.Mock
}

func (m *mockAWSSNSClient) Publish(input *sns.PublishInput) (*sns.PublishOutput, error) {
	args := m.Called(input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sns.PublishOutput), args.Error(1)
}

func Test_NewAWSSNSClient(t *testing.T) {
	// Declare types in advance to make sure these are the types being returned
	var gotAWSSNSClient *awsSNSClient
	var err error

	// accessKeyID cannot be empty
	gotAWSSNSClient, err = NewAWSSNSClient("", "", "", "")
	require.Nil(t, gotAWSSNSClient)
	require.EqualError(t, err, "aws accessKeyID is empty")

	// secretAccessKey cannot be empty
	gotAWSSNSClient, err = NewAWSSNSClient("accessKeyID", "", "", "")
	require.Nil(t, gotAWSSNSClient)
	require.EqualError(t, err, "aws secretAccessKey is empty")

	// region cannot be empty
	gotAWSSNSClient, err = NewAWSSNSClient("accessKeyID", "secretAccessKey", "", "")
	require.Nil(t, gotAWSSNSClient)
	require.EqualError(t, err, "aws region is empty")

	// [sms] type doesn't need a sender ID:
	gotAWSSNSClient, err = NewAWSSNSClient("accessKeyID", "secretAccessKey", "region", "  ")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSNSClient)

	// [sms] all fields are present 🎉
	gotAWSSNSClient, err = NewAWSSNSClient("accessKeyID", "secretAccessKey", "region", "testSenderID")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSNSClient)
}

func Test_AWSSNS_SendMessage_messageIsInvalid(t *testing.T) {
	var mAWS MessengerClient = &awsSNSClient{}
	err := mAWS.SendMessage(Message{})
	require.EqualError(t, err, "validating message to send an SMS through AWS: invalid message: phone number cannot be empty")
}

func Test_AWSSNS_SendMessage_errorIsHandledCorrectly(t *testing.T) {
	// check if error is handled correctly
	testPhoneNumber := "+14155555555"
	testMessage := "foo bar"
	testSenderID := "senderID"
	mAWSSNS := mockAWSSNSClient{}
	mAWSSNS.
		On("Publish", &sns.PublishInput{
			PhoneNumber: aws.String(testPhoneNumber),
			Message:     aws.String(testMessage),
			MessageAttributes: map[string]*sns.MessageAttributeValue{
				"AWS.SNS.SMS.SenderID": {StringValue: aws.String(testSenderID), DataType: aws.String("String")},
				"AWS.SNS.SMS.SMSType":  {StringValue: aws.String("Transactional"), DataType: aws.String("String")},
			},
		}).
		Return(nil, fmt.Errorf("test AWS SNS error")).
		Once()

	mAWS := awsSNSClient{snsService: &mAWSSNS, senderID: "senderID"}
	err := mAWS.SendMessage(Message{ToPhoneNumber: "+14155555555", Message: "foo bar"})
	require.EqualError(t, err, "sending AWS SNS SMS: test AWS SNS error")

	mAWSSNS.AssertExpectations(t)
}

func Test_AWSSNS_SendMessage_success(t *testing.T) {
	// check if error is handled correctly
	testPhoneNumber := "+14152222222"
	testMessage := "foo bar"
	testSenderID := "senderID"
	mAWSSNS := mockAWSSNSClient{}
	mAWSSNS.
		On("Publish", &sns.PublishInput{
			PhoneNumber: aws.String(testPhoneNumber),
			Message:     aws.String(testMessage),
			MessageAttributes: map[string]*sns.MessageAttributeValue{
				"AWS.SNS.SMS.SenderID": {StringValue: aws.String(testSenderID), DataType: aws.String("String")},
				"AWS.SNS.SMS.SMSType":  {StringValue: aws.String("Transactional"), DataType: aws.String("String")},
			},
		}).
		Return(nil, nil).
		Once()

	mAWS := awsSNSClient{snsService: &mAWSSNS, senderID: "senderID"}
	err := mAWS.SendMessage(Message{ToPhoneNumber: "+14152222222", Message: "foo bar"})
	require.NoError(t, err)

	mAWSSNS.AssertExpectations(t)
}
