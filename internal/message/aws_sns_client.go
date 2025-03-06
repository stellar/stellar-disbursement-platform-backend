package message

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// awsSNSInterface is used to send sms.
type awsSNSInterface interface {
	Publish(input *sns.PublishInput) (*sns.PublishOutput, error)
}

// awsSNSClient is used to send sms.
type awsSNSClient struct {
	snsService awsSNSInterface
	senderID   string
}

func (t *awsSNSClient) MessengerType() MessengerType {
	return MessengerTypeAWSSMS
}

func (a *awsSNSClient) SendMessage(message Message) error {
	err := message.ValidateFor(a.MessengerType())
	if err != nil {
		return fmt.Errorf("validating message to send an SMS through AWS: %w", err)
	}

	messageAttributes := map[string]*sns.MessageAttributeValue{
		"AWS.SNS.SMS.SMSType": {StringValue: aws.String("Transactional"), DataType: aws.String("String")},
	}
	if a.senderID != "" {
		// According with AWS, senderID is optional: https://docs.aws.amazon.com/sns/latest/dg/sms_publish-to-phone.html#sms_publish_sdk
		messageAttributes["AWS.SNS.SMS.SenderID"] = &sns.MessageAttributeValue{StringValue: aws.String(a.senderID), DataType: aws.String("String")}
	}

	params := &sns.PublishInput{
		PhoneNumber:       aws.String(message.ToPhoneNumber),
		Message:           aws.String(message.Body),
		MessageAttributes: messageAttributes,
	}

	_, err = a.snsService.Publish(params)
	if err != nil {
		return fmt.Errorf("sending AWS SNS SMS: %w", err)
	}

	log.Debugf("🎉 AWS SNS sent an SMS to the phoneNumber %q", utils.TruncateString(message.ToPhoneNumber, 3))
	return nil
}

// NewAWSSNSClient creates a new awsSNSClient, that is used to send SMS messages.
func NewAWSSNSClient(accessKeyID, secretAccessKey, region, senderID string) (*awsSNSClient, error) {
	accessKeyID = strings.TrimSpace(accessKeyID)
	secretAccessKey = strings.TrimSpace(secretAccessKey)
	region = strings.TrimSpace(region)

	awsConfig := aws.Config{}
	if accessKeyID != "" && secretAccessKey != "" && region != "" {
		log.Debug("Using SDP custom AWS static credential configuration")
		awsConfig.Credentials = credentials.NewStaticCredentials(accessKeyID, secretAccessKey, "")
		awsConfig.Region = aws.String(region)
	}

	awsSession, err := session.NewSession(&awsConfig)
	if err != nil {
		return nil, fmt.Errorf("creating AWS session: %w", err)
	}

	senderID = strings.TrimSpace(senderID)

	return &awsSNSClient{
		senderID:   senderID,
		snsService: sns.New(awsSession),
	}, nil
}

var _ MessengerClient = (*awsSNSClient)(nil)
