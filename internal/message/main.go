package message

import (
	"fmt"
	"strings"

	"golang.org/x/exp/slices"
)

type MessengerType string

// ATTENTION: when adding a new type, make ure to update the MessengerType methods!
const (
	// MessengerTypeTwilioSMS is used to send SMS messages using Twilio.
	MessengerTypeTwilioSMS MessengerType = "TWILIO_SMS"
	// MessengerTypeAWSSMS is used to send SMS messages using AWS SNS.
	MessengerTypeAWSSMS MessengerType = "AWS_SMS"
	// MessengerTypeAWSEmail is used to send emails using AWS SES.
	MessengerTypeAWSEmail MessengerType = "AWS_EMAIL"
	// MessengerTypeDryRun is used for development environment
	MessengerTypeDryRun MessengerType = "DRY_RUN"
)

func (mt MessengerType) All() []MessengerType {
	return []MessengerType{MessengerTypeTwilioSMS, MessengerTypeAWSSMS, MessengerTypeAWSEmail, MessengerTypeDryRun}
}

func ParseMessengerType(messengerTypeStr string) (MessengerType, error) {
	messageTypeStrUpper := strings.ToUpper(messengerTypeStr)
	mType := MessengerType(messageTypeStrUpper)

	if slices.Contains(MessengerType("").All(), mType) {
		return mType, nil
	}

	return "", fmt.Errorf("invalid message sender type %q", messageTypeStrUpper)
}

func (mt MessengerType) ValidSMSTypes() []MessengerType {
	return []MessengerType{MessengerTypeDryRun, MessengerTypeTwilioSMS, MessengerTypeAWSSMS}
}

func (mt MessengerType) ValidEmailTypes() []MessengerType {
	return []MessengerType{MessengerTypeDryRun, MessengerTypeAWSEmail}
}

func (mt MessengerType) IsSMS() bool {
	return slices.Contains(mt.ValidSMSTypes(), mt)
}

func (mt MessengerType) IsEmail() bool {
	return slices.Contains(mt.ValidEmailTypes(), mt)
}

type MessengerOptions struct {
	MessengerType MessengerType
	Environment   string

	// Twilio
	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioServiceSID string

	// AWS
	AWSAccessKeyID     string
	AWSSecretAccessKey string
	AWSRegion          string
	// AWS SNS (SMS messages)
	AWSSNSSenderID string
	// AWS SES (EMAIL messages)
	AWSSESSenderID string
}

func GetClient(opts MessengerOptions) (MessengerClient, error) {
	switch opts.MessengerType {
	case MessengerTypeTwilioSMS:
		return NewTwilioClient(opts.TwilioAccountSID, opts.TwilioAuthToken, opts.TwilioServiceSID)

	case MessengerTypeAWSSMS:
		return NewAWSSNSClient(opts.AWSAccessKeyID, opts.AWSSecretAccessKey, opts.AWSRegion, opts.AWSSNSSenderID)

	case MessengerTypeAWSEmail:
		return NewAWSSESClient(opts.AWSAccessKeyID, opts.AWSSecretAccessKey, opts.AWSRegion, opts.AWSSESSenderID)

	case MessengerTypeDryRun:
		return NewDryRunClient()

	default:
		return nil, fmt.Errorf("unknown message sender type: %q", opts.MessengerType)
	}
}
