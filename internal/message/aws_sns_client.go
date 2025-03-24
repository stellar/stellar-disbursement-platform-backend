package message

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// awsSNSInterface is used to send SMS.
type awsSNSInterface interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

// awsSNSClient is used to send SMS.
type awsSNSClient struct {
	snsService awsSNSInterface
	senderID   string
}

func (t *awsSNSClient) MessengerType() MessengerType {
	return MessengerTypeAWSSMS
}

func (a *awsSNSClient) SendMessage(ctx context.Context, message Message) error {
	err := message.ValidateFor(a.MessengerType())
	if err != nil {
		return fmt.Errorf("validating message to send an SMS through AWS: %w", err)
	}

	messageAttributes := map[string]types.MessageAttributeValue{
		"AWS.SNS.SMS.SMSType": {
			StringValue: aws.String("Transactional"),
			DataType:    aws.String("String"),
		},
	}
	if a.senderID != "" {
		// SenderID is optional per AWS docs: https://docs.aws.amazon.com/sns/latest/dg/sms_publish-to-phone.html#sms_publish_sdk
		messageAttributes["AWS.SNS.SMS.SenderID"] = types.MessageAttributeValue{
			StringValue: aws.String(a.senderID),
			DataType:    aws.String("String"),
		}
	}

	params := &sns.PublishInput{
		PhoneNumber:       aws.String(message.ToPhoneNumber),
		Message:           aws.String(message.Body),
		MessageAttributes: messageAttributes,
	}

	_, err = a.snsService.Publish(ctx, params)
	if err != nil {
		return fmt.Errorf("sending AWS SNS SMS: %w", err)
	}

	log.Debugf("🎉 AWS SNS sent an SMS to the phoneNumber %q", utils.TruncateString(message.ToPhoneNumber, 3))
	return nil
}

// NewAWSSNSClient creates a new awsSNSClient, that is used to send SMS messages.
func NewAWSSNSClient(accessKeyID, secretAccessKey, region, senderID string) (*awsSNSClient, error) {
	senderID = strings.TrimSpace(senderID)

	cfg, err := loadAWSConfig(accessKeyID, secretAccessKey, region)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config for SNS: %w", err)
	}

	snsClient := sns.NewFromConfig(cfg)

	return &awsSNSClient{
		senderID:   senderID,
		snsService: snsClient,
	}, nil
}

var _ MessengerClient = (*awsSNSClient)(nil)
