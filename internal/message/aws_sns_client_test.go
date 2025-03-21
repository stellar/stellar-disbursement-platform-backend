package message

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockAWSSNSClient struct {
	mock.Mock
}

func (m *mockAWSSNSClient) Publish(ctx context.Context, input *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	inputArgs := []interface{}{ctx, input}
	for _, optFn := range optFns {
		inputArgs = append(inputArgs, optFn)
	}
	args := m.Called(inputArgs...)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sns.PublishOutput), args.Error(1)
}

func Test_NewAWSSNSClient(t *testing.T) {
	// Declare types in advance to make sure these are the types being returned
	var gotAWSSNSClient *awsSNSClient
	var err error

	// accessKeyID can be empty
	gotAWSSNSClient, err = NewAWSSNSClient("", "", "", "")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSNSClient)

	// secretAccessKey can be empty
	gotAWSSNSClient, err = NewAWSSNSClient("accessKeyID", "", "", "")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSNSClient)

	// region can be empty
	gotAWSSNSClient, err = NewAWSSNSClient("accessKeyID", "secretAccessKey", "", "")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSNSClient)

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
	err := mAWS.SendMessage(context.Background(), Message{})
	require.EqualError(t, err, "validating message to send an SMS through AWS: invalid message: phone number cannot be empty")
}

func Test_AWSSNS_SendMessage_errorIsHandledCorrectly(t *testing.T) {
	// check if error is handled correctly
	testPhoneNumber := "+14155555555"
	testMessage := "foo bar"
	testSenderID := "senderID"
	mAWSSNS := mockAWSSNSClient{}
	mAWSSNS.
		On("Publish", mock.Anything, &sns.PublishInput{
			PhoneNumber: aws.String(testPhoneNumber),
			Message:     aws.String(testMessage),
			MessageAttributes: map[string]types.MessageAttributeValue{
				"AWS.SNS.SMS.SenderID": {StringValue: aws.String(testSenderID), DataType: aws.String("String")},
				"AWS.SNS.SMS.SMSType":  {StringValue: aws.String("Transactional"), DataType: aws.String("String")},
			},
		}).
		Return(nil, fmt.Errorf("test AWS SNS error")).
		Once()

	mAWS := awsSNSClient{snsService: &mAWSSNS, senderID: "senderID"}
	err := mAWS.SendMessage(context.Background(), Message{ToPhoneNumber: "+14155555555", Body: "foo bar"})
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
		On("Publish", mock.Anything, &sns.PublishInput{
			PhoneNumber: aws.String(testPhoneNumber),
			Message:     aws.String(testMessage),
			MessageAttributes: map[string]types.MessageAttributeValue{
				"AWS.SNS.SMS.SenderID": {StringValue: aws.String(testSenderID), DataType: aws.String("String")},
				"AWS.SNS.SMS.SMSType":  {StringValue: aws.String("Transactional"), DataType: aws.String("String")},
			},
		}).
		Return(nil, nil).
		Once()

	mAWS := awsSNSClient{snsService: &mAWSSNS, senderID: "senderID"}
	err := mAWS.SendMessage(context.Background(), Message{ToPhoneNumber: "+14152222222", Body: "foo bar"})
	require.NoError(t, err)

	mAWSSNS.AssertExpectations(t)
}
