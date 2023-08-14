package utils

import (
	"go/types"

	"github.com/stellar/go/support/config"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

// TwilioConfigOptions returns the config options for Twilio. Relevant for loading configs needed for the messenger type(s): `TWILIO_*`.
func TwilioConfigOptions(opts *message.MessengerOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		{
			Name:      "twilio-account-sid",
			Usage:     "The SID of the Twilio account",
			OptType:   types.String,
			ConfigKey: &opts.TwilioAccountSID,
			Required:  false,
		},
		{
			Name:      "twilio-auth-token",
			Usage:     "The Auth Token of the Twilio account",
			OptType:   types.String,
			ConfigKey: &opts.TwilioAuthToken,
			Required:  false,
		},
		{
			Name:      "twilio-service-sid",
			Usage:     "The service ID used within Twilio to send messages",
			OptType:   types.String,
			ConfigKey: &opts.TwilioServiceSID,
			Required:  false,
		},
	}
}

// AWSConfigOptions returns the config options for AWS. Relevant for loading configs needed for the messenger type(s): `AWS_*`.
func AWSConfigOptions(opts *message.MessengerOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		// AWS
		{
			Name:      "aws-access-key-id",
			Usage:     "The AWS access key ID",
			OptType:   types.String,
			ConfigKey: &opts.AWSAccessKeyID,
			Required:  false,
		},
		{
			Name:      "aws-secret-access-key",
			Usage:     "The AWS secret access key",
			OptType:   types.String,
			ConfigKey: &opts.AWSSecretAccessKey,
			Required:  false,
		},
		{
			Name:      "aws-region",
			Usage:     "The AWS region",
			OptType:   types.String,
			ConfigKey: &opts.AWSRegion,
			Required:  false,
		},
		// AWS SMS (SNS)
		{
			Name:      "aws-sns-sender-id",
			Usage:     "The sender ID of the aws account sending the SMS message. Uses AWS SNS.",
			OptType:   types.String,
			ConfigKey: &opts.AWSSNSSenderID,
			Required:  false,
		},
		// AWS Email (SES)
		{
			Name:      "aws-ses-sender-id",
			Usage:     "The email address that AWS will use to send emails. Uses AWS SES.",
			OptType:   types.String,
			ConfigKey: &opts.AWSSESSenderID,
			Required:  false,
		},
	}
}
