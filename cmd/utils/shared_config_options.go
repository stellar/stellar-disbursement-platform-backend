package utils

import (
	"fmt"
	"go/types"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/events"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
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

type TenantRoutingOptions struct {
	All      bool
	TenantID string
}

func (o *TenantRoutingOptions) ValidateFlags() error {
	if !o.All && o.TenantID == "" {
		return fmt.Errorf(
			"invalid config. Please specify --all to run the migrations for all tenants " +
				"or specify --tenant-id to run the migrations to a specific tenant",
		)
	}
	return nil
}

// TenantRoutingConfigOptions returns the config options for routing commands that apply to all tenants or a specific tenant.
func TenantRoutingConfigOptions(opts *TenantRoutingOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		{
			Name:        "all",
			Usage:       "Apply the command to all tenants. Either --tenant-id or --all must be set, but the --all option will be ignored if --tenant-id is set.",
			OptType:     types.Bool,
			FlagDefault: false,
			ConfigKey:   &opts.All,
			Required:    false,
		},
		{
			Name:      "tenant-id",
			Usage:     "The tenant ID where the command will be applied. Either --tenant-id or --all must be set, but the --all option will be ignored if --tenant-id is set.",
			OptType:   types.String,
			ConfigKey: &opts.TenantID,
			Required:  false,
		},
	}
}

type EventBrokerOptions struct {
	EventBrokerType events.EventBrokerType
	BrokerURLs      []string
	ConsumerGroupID string

	// KAFKA specific options
	KafkaSecurityProtocol events.KafkaSecurityProtocol
	KafkaSASLUsername     string
	KafkaSASLPassword     string
}

func EventBrokerConfigOptions(opts *EventBrokerOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		{
			Name:           "event-broker-type",
			Usage:          `Event Broker type. Options: "KAFKA", "NONE"`,
			OptType:        types.String,
			ConfigKey:      &opts.EventBrokerType,
			CustomSetValue: SetConfigOptionEventBrokerType,
			FlagDefault:    string(events.KafkaEventBrokerType),
			Required:       true,
		},
		{
			Name:           "broker-urls",
			Usage:          "List of Message Broker URLs comma separated.",
			OptType:        types.String,
			ConfigKey:      &opts.BrokerURLs,
			CustomSetValue: SetConfigOptionURLList,
			Required:       false,
		},
		{
			Name:      "consumer-group-id",
			Usage:     "Message Broker Consumer Group ID.",
			OptType:   types.String,
			ConfigKey: &opts.ConsumerGroupID,
			Required:  false,
		},

		{
			Name:           "kafka-security-protocol",
			Usage:          "Kafka Security Protocol. Options: PLAINTEXT, SASL_PLAINTEXT, SASL_SSL, SSL",
			OptType:        types.String,
			CustomSetValue: SetConfigOptionKafkaSecurityProtocol,
			ConfigKey:      &opts.KafkaSecurityProtocol,
			Required:       true,
		},
		{
			Name:      "kafka-sasl-username",
			Usage:     "Kafka SASL Username",
			OptType:   types.String,
			ConfigKey: &opts.KafkaSASLUsername,
			Required:  false,
		},
		{
			Name:      "kafka-sasl-password",
			Usage:     "Kafka SASL Password",
			OptType:   types.String,
			ConfigKey: &opts.KafkaSASLPassword,
			Required:  false,
		},
	}
}

func TransactionSubmitterEngineConfigOptions(opts *di.TxSubmitterEngineOptions) config.ConfigOptions {
	return append(
		BaseSignatureServiceConfigOptions(&opts.SignatureServiceOptions),
		&config.ConfigOption{
			Name:        "max-base-fee",
			Usage:       "The max base fee for submitting a Stellar transaction",
			OptType:     types.Int,
			ConfigKey:   &opts.MaxBaseFee,
			FlagDefault: 100 * txnbuild.MinBaseFee,
			Required:    true,
		},
		&config.ConfigOption{
			Name:        "horizon-url",
			Usage:       "The URL of the Stellar Horizon server where this application will communicate with.",
			OptType:     types.String,
			ConfigKey:   &opts.HorizonURL,
			FlagDefault: horizonclient.DefaultTestNetClient.HorizonURL,
			Required:    true,
		},
	)
}

func BaseSignatureServiceConfigOptions(opts *signing.SignatureServiceOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		{
			Name:           "channel-account-encryption-passphrase",
			Usage:          "A Stellar-compliant ed25519 private key used to encrypt/decrypt the channel accounts' private keys. When not set, it will default to the value of the 'distribution-seed' option.",
			OptType:        types.String,
			CustomSetValue: SetConfigOptionStellarPrivateKey,
			ConfigKey:      &opts.ChAccEncryptionPassphrase,
			Required:       false,
		},
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the Stellar distribution account that sends the disbursements.", // TODO: this will eventually be used for sponsoring tenant accounts.
			OptType:        types.String,
			CustomSetValue: SetConfigOptionStellarPrivateKey,
			ConfigKey:      &opts.DistributionPrivateKey,
			Required:       true,
		},
		{
			Name:           "distribution-signer-type",
			Usage:          fmt.Sprintf("The type of the signature client used for distribution accounts. Options: %+v", signing.DistributionSignatureClientTypes()),
			OptType:        types.String,
			CustomSetValue: SetConfigOptionDistributionSignerType,
			ConfigKey:      &opts.DistributionSignerType,
			FlagDefault:    string(signing.DistributionAccountEnvSignatureClientType),
			Required:       true,
		},
	}
}

func CrashTrackerTypeConfigOption(targetPointer interface{}) *config.ConfigOption {
	return &config.ConfigOption{
		Name:           "crash-tracker-type",
		Usage:          `Crash tracker type. Options: "SENTRY", "DRY_RUN"`,
		OptType:        types.String,
		CustomSetValue: SetConfigOptionCrashTrackerType,
		ConfigKey:      targetPointer,
		FlagDefault:    string(crashtracker.CrashTrackerTypeDryRun),
		Required:       true,
	}
}

func DistributionPublicKey(targetPointer interface{}) *config.ConfigOption {
	return &config.ConfigOption{
		Name:           "distribution-public-key",
		Usage:          "The public key of the Stellar distribution account that sends the disbursements.",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionStellarPublicKey,
		ConfigKey:      targetPointer,
		Required:       true,
	}
}

func KafkaConfig(opts EventBrokerOptions) events.KafkaConfig {
	return events.KafkaConfig{
		Brokers:          opts.BrokerURLs,
		SecurityProtocol: opts.KafkaSecurityProtocol,
		SASLUsername:     opts.KafkaSASLUsername,
		SASLPassword:     opts.KafkaSASLPassword,
	}
}
