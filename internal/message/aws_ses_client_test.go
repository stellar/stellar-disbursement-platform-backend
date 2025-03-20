package message

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockAWSSESClient struct {
	mock.Mock
}

func (m *mockAWSSESClient) SendEmail(ctx context.Context, input *ses.SendEmailInput, optFns ...func(*ses.Options)) (*ses.SendEmailOutput, error) {
	inputArgs := []interface{}{ctx, input}
	for _, optFn := range optFns {
		inputArgs = append(inputArgs, optFn)
	}
	args := m.Called(inputArgs...)

	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ses.SendEmailOutput), args.Error(1)
}

func Test_NewAWSSESClient(t *testing.T) {
	// Declare types in advance to make sure these are the types being returned
	var gotAWSSESClient *awsSESClient
	var err error

	// accessKeyID can be empty
	gotAWSSESClient, err = NewAWSSESClient("", "", "", "foo@test.com")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSESClient)

	// secretAccessKey can be empty
	gotAWSSESClient, err = NewAWSSESClient("accessKeyID", "", "", "foo@test.com")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSESClient)

	// region can be empty
	gotAWSSESClient, err = NewAWSSESClient("accessKeyID", "secretAccessKey", "", "foo@test.com")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSESClient)

	// [email] type needs a valid email as a sender ID:
	gotAWSSESClient, err = NewAWSSESClient("accessKeyID", "secretAccessKey", "region", "invalid-email")
	require.Nil(t, gotAWSSESClient)
	require.EqualError(t, err, "aws SES (email) senderID is invalid: the email address provided is not valid")

	// [email] all fields are present 🎉
	gotAWSSESClient, err = NewAWSSESClient("accessKeyID", "secretAccessKey", "region", "foo@test.com")
	require.NoError(t, err)
	require.NotNil(t, gotAWSSESClient)
}

func Test_AWSSES_SendMessage_messageIsInvalid(t *testing.T) {
	var mAWS MessengerClient = &awsSESClient{}
	err := mAWS.SendMessage(Message{})
	require.EqualError(t, err, "validating message to send an email through AWS: invalid e-mail: invalid email format: email field is required")
}

func Test_AWSSES_SendMessage_errorIsHandledCorrectly(t *testing.T) {
	testSenderID := "sender@test.com"
	message := Message{ToEmail: "foo@test.com", Title: "test title", Body: "foo bar"}
	emailStr, err := generateAWSEmail(message, testSenderID)
	require.NoError(t, err)

	mAWSSES := mockAWSSESClient{}
	mAWSSES.
		On("SendEmail", mock.Anything, emailStr).
		Return(nil, fmt.Errorf("test AWS SES error")).
		Once()

	mAWS := awsSESClient{emailService: &mAWSSES, senderID: "sender@test.com"}
	err = mAWS.SendMessage(Message{ToEmail: "foo@test.com", Title: "test title", Body: "foo bar"})
	require.EqualError(t, err, "sending AWS SES email: test AWS SES error")

	mAWSSES.AssertExpectations(t)
}

func Test_AWSSES_SendMessage_success(t *testing.T) {
	testSenderID := "sender@test.com"
	message := Message{ToEmail: "foo@test.com", Title: "test title", Body: "foo bar"}
	emailStr, err := generateAWSEmail(message, testSenderID)
	require.NoError(t, err)

	mAWSSES := mockAWSSESClient{}
	mAWSSES.
		On("SendEmail", mock.Anything, emailStr).
		Return(nil, nil).
		Once()

	mAWS := awsSESClient{emailService: &mAWSSES, senderID: "sender@test.com"}
	err = mAWS.SendMessage(Message{ToEmail: "foo@test.com", Title: "test title", Body: "foo bar"})
	require.NoError(t, err)

	mAWSSES.AssertExpectations(t)
}

func Test_generateAWSEmail_success(t *testing.T) {
	message := Message{
		ToEmail: "receiver@test.com",
		Body:    "Helo world!",
		Title:   "title",
	}
	gotEmail, err := generateAWSEmail(message, "sender@test.com")
	require.NoError(t, err)

	wantHTML := `<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<meta http-equiv="X-UA-Compatible" content="IE=edge,chrome=1">
	</head>
	<body>
	Helo world!
	</body>
	</html>`
	wantHTML = strings.TrimSpace(wantHTML)
	// remove tabs:
	wantHTML = strings.ReplaceAll(wantHTML, "\t\t", "    ")
	wantHTML = strings.ReplaceAll(wantHTML, "\t", "")

	wantEmail := &ses.SendEmailInput{
		Destination: &types.Destination{
			ToAddresses: []string{message.ToEmail},
		},
		Message: &types.Message{
			Body: &types.Body{
				Html: &types.Content{
					Charset: aws.String("utf-8"),
					Data:    aws.String(wantHTML),
				},
			},
			Subject: &types.Content{
				Charset: aws.String("utf-8"),
				Data:    aws.String("title"),
			},
		},
		Source: aws.String("sender@test.com"),
	}
	require.Equal(t, wantEmail, gotEmail)
}
