package utils

import (
	"fmt"
	"go/types"
	"strings"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/crashtracker"
	di "github.com/stellar/stellar-disbursement-platform-backend/internal/dependencyinjection"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/scheduler"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

// DBPoolOptions contains tunables for the PostgreSQL connection pool.
type DBPoolOptions struct {
	DBMaxOpenConns           int
	DBMaxIdleConns           int
	DBConnMaxIdleTimeSeconds int
	DBConnMaxLifetimeSeconds int
}

// DBPoolConfigOptions returns config options for tuning the DB connection pool.
func DBPoolConfigOptions(opts *DBPoolOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		{
			Name:        "db-max-open-conns",
			Usage:       "Maximum number of open DB connections per pool",
			OptType:     types.Int,
			ConfigKey:   &opts.DBMaxOpenConns,
			FlagDefault: db.DefaultDBPoolConfig.MaxOpenConns,
			Required:    false,
		},
		{
			Name:        "db-max-idle-conns",
			Usage:       "Maximum number of idle DB connections retained per pool",
			OptType:     types.Int,
			ConfigKey:   &opts.DBMaxIdleConns,
			FlagDefault: db.DefaultDBPoolConfig.MaxIdleConns,
			Required:    false,
		},
		{
			Name:        "db-conn-max-idle-time-seconds",
			Usage:       "Maximum idle time in seconds before a connection is closed",
			OptType:     types.Int,
			ConfigKey:   &opts.DBConnMaxIdleTimeSeconds,
			FlagDefault: db.DefaultDBPoolConfig.ConnMaxIdleTime,
			Required:    false,
		},
		{
			Name:        "db-conn-max-lifetime-seconds",
			Usage:       "Maximum lifetime in seconds for a single connection",
			OptType:     types.Int,
			ConfigKey:   &opts.DBConnMaxLifetimeSeconds,
			FlagDefault: db.DefaultDBPoolConfig.ConnMaxLifetime,
			Required:    false,
		},
	}
}

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
		// Twilio WhatsApp
		{
			Name:      "twilio-whatsapp-from-number",
			Usage:     "The WhatsApp Business number used to send messages (with whatsapp: prefix)",
			OptType:   types.String,
			ConfigKey: &opts.TwilioWhatsAppFromNumber,
			Required:  false,
		},
		{
			Name:      "twilio-whatsapp-receiver-invitation-template-sid",
			Usage:     "The Twilio Content SID for WhatsApp receiver invitation template (starts with HX)",
			OptType:   types.String,
			ConfigKey: &opts.TwilioWhatsAppReceiverInvitationTemplateSID,
			Required:  false,
		},
		{
			Name:      "twilio-whatsapp-receiver-otp-template-sid",
			Usage:     "The Twilio Content SID for WhatsApp receiver OTP template (starts with HX)",
			OptType:   types.String,
			ConfigKey: &opts.TwilioWhatsAppReceiverOTPTemplateSID,
			Required:  false,
		},
		// Twilio Email (SendGrid)
		{
			Name:      "twilio-sendgrid-api-key",
			Usage:     "The API key of the Twilio SendGrid account",
			OptType:   types.String,
			ConfigKey: &opts.TwilioSendGridAPIKey,
			Required:  false,
		},
		{
			Name:      "twilio-sendgrid-sender-address",
			Usage:     "The email address that Twilio SendGrid will use to send emails",
			OptType:   types.String,
			ConfigKey: &opts.TwilioSendGridSenderAddress,
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
		SingleTenantRoutingConfigOptions(opts),
	}
}

func SingleTenantRoutingConfigOptions(opts *TenantRoutingOptions) *config.ConfigOption {
	return &config.ConfigOption{
		Name:      "tenant-id",
		Usage:     "The tenant ID where the command will be applied.",
		OptType:   types.String,
		ConfigKey: &opts.TenantID,
		Required:  false,
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
		HorizonURL(&opts.HorizonURL),
	)
}

func BaseSignatureServiceConfigOptions(opts *signing.SignatureServiceOptions) []*config.ConfigOption {
	return append([]*config.ConfigOption{
		{
			Name:           "channel-account-encryption-passphrase",
			Usage:          "A Stellar-compliant ed25519 private key used to encrypt/decrypt the channel accounts' private keys. When not set, it will default to the value of the 'distribution-seed' option.",
			OptType:        types.String,
			CustomSetValue: SetConfigOptionStellarPrivateKey,
			ConfigKey:      &opts.ChAccEncryptionPassphrase,
			Required:       false,
		},
	}, BaseDistributionAccountSignatureClientConfigOptions(opts)...)
}

func BaseDistributionAccountSignatureClientConfigOptions(opts *signing.SignatureServiceOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		{
			Name:           "distribution-account-encryption-passphrase",
			Usage:          "A Stellar-compliant ed25519 private key used to encrypt and decrypt the private keys of tenants' distribution accounts.",
			OptType:        types.String,
			CustomSetValue: SetConfigOptionStellarPrivateKey,
			ConfigKey:      &opts.DistAccEncryptionPassphrase,
			Required:       true,
		},
		{
			Name:           "distribution-seed",
			Usage:          "The private key of the HOST's Stellar distribution account, used to create channel accounts",
			OptType:        types.String,
			CustomSetValue: SetConfigOptionStellarPrivateKey,
			ConfigKey:      &opts.DistributionPrivateKey,
			Required:       true,
		},
	}
}

func TenantXLMBootstrapAmount(targetPointer interface{}) *config.ConfigOption {
	return &config.ConfigOption{
		Name:        "tenant-xlm-bootstrap-amount",
		Usage:       "The amount of the native asset that will be sent to the tenant distribution account from the host distribution account when it's created if applicable.",
		OptType:     types.Int,
		ConfigKey:   targetPointer,
		FlagDefault: tenant.MinTenantDistributionAccountAmount,
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

func SchedulerConfigOptions(opts *scheduler.SchedulerOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		{
			Name:        "scheduler-payment-job-seconds",
			Usage:       fmt.Sprintf("The interval in seconds for the payment jobs that synchronize transactions between SDP and TSS. Must be greater than %d seconds.", 5),
			OptType:     types.Int,
			ConfigKey:   &opts.PaymentJobIntervalSeconds,
			FlagDefault: 30,
			Required:    false,
		},
		{
			Name:        "scheduler-receiver-invitation-job-seconds",
			Usage:       fmt.Sprintf("The interval in seconds for the receiver invitation job that sends invitations to new receivers. Must be greater than %d seconds.", 5),
			OptType:     types.Int,
			ConfigKey:   &opts.ReceiverInvitationJobIntervalSeconds,
			FlagDefault: 30,
			Required:    false,
		},
	}
}

type BridgeIntegrationOptions struct {
	EnableBridgeIntegration bool
	BridgeBaseURL           string
	BridgeAPIKey            string
}

func (opts *BridgeIntegrationOptions) ValidateFlags() error {
	if !opts.EnableBridgeIntegration {
		return nil
	}

	if strings.TrimSpace(opts.BridgeAPIKey) == "" {
		return fmt.Errorf("bridge API key must be set when bridge integration is enabled")
	}
	if strings.TrimSpace(opts.BridgeBaseURL) == "" {
		return fmt.Errorf("bridge base URL must be set when bridge integration is enabled")
	}
	isBaseURL, err := utils.IsBaseURL(opts.BridgeBaseURL)
	if err != nil {
		return fmt.Errorf("validating bridge base URL: %w", err)
	}
	if !isBaseURL {
		return fmt.Errorf("bridge base URL must be a base URL e.g. `https://api.bridge.xyz`")
	}

	return nil
}

func BridgeIntegrationConfigOptions(opts *BridgeIntegrationOptions) []*config.ConfigOption {
	return []*config.ConfigOption{
		{
			Name:        "enable-bridge-integration",
			Usage:       "Enable Bridge integration for Liquidity Sourcing.",
			OptType:     types.Bool,
			ConfigKey:   &opts.EnableBridgeIntegration,
			FlagDefault: false,
			Required:    false,
		},
		{
			Name:        "bridge-base-url",
			Usage:       "Bridge Base URL. This needs to be configured only if the Bridge integration is enabled.",
			OptType:     types.String,
			ConfigKey:   &opts.BridgeBaseURL,
			FlagDefault: "https://api.bridge.xyz",
			Required:    false,
		},
		{
			Name:      "bridge-api-key",
			Usage:     "Bridge API key. This needs to be configured only if the Bridge integration is enabled.",
			OptType:   types.String,
			ConfigKey: &opts.BridgeAPIKey,
			Required:  false,
		},
	}
}

func DistributionPublicKey(targetPointer interface{}) *config.ConfigOption {
	return &config.ConfigOption{
		Name:           "distribution-public-key",
		Usage:          "The public key of the HOST's Stellar distribution account, used to create channel accounts",
		OptType:        types.String,
		CustomSetValue: SetConfigOptionStellarPublicKey,
		ConfigKey:      targetPointer,
		Required:       true,
	}
}

func NetworkPassphrase(targetPointer interface{}) *config.ConfigOption {
	return &config.ConfigOption{
		Name:        "network-passphrase",
		Usage:       "The Stellar network passphrase",
		OptType:     types.String,
		FlagDefault: network.TestNetworkPassphrase,
		ConfigKey:   targetPointer,
		Required:    true,
	}
}

func HorizonURL(targetPointer interface{}) *config.ConfigOption {
	return &config.ConfigOption{
		Name:        "horizon-url",
		Usage:       "The URL of the Stellar Horizon server where this application will communicate with.",
		OptType:     types.String,
		ConfigKey:   targetPointer,
		FlagDefault: horizonclient.DefaultTestNetClient.HorizonURL,
		Required:    true,
	}
}
