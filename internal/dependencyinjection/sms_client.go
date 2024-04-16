package dependencyinjection

import (
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

const SMSClientInstanceName = "sms_client_instance"

type SMSClientOptions struct {
	SMSType          message.MessengerType
	MessengerOptions *message.MessengerOptions
}

// buildSMSClientInstanceName creates a new SMS client instance, or retrives a instance that was already created before.
func buildSMSClientInstanceName(smsClientType message.MessengerType) string {
	return fmt.Sprintf("%s-%s", SMSClientInstanceName, string(smsClientType))
}

// NewSMSClient creates a new SMS client instance, or retrives a instance that
// was already created before.
func NewSMSClient(opts SMSClientOptions) (message.MessengerClient, error) {
	if !opts.SMSType.IsSMS() {
		return nil, fmt.Errorf("trying to create a SMS client with a non-supported SMS type: %q", opts.SMSType)
	}

	if opts.MessengerOptions == nil {
		opts.MessengerOptions = &message.MessengerOptions{}
	}
	opts.MessengerOptions.MessengerType = opts.SMSType

	// If there is already an instance of the service, we return the same instance
	instanceName := buildSMSClientInstanceName(opts.MessengerOptions.MessengerType)
	if instance, ok := GetInstance(instanceName); ok {
		if smsClientInstance, ok := instance.(message.MessengerClient); ok {
			return smsClientInstance, nil
		}
		return nil, fmt.Errorf("trying to cast pre-existing SMS client for depencency injection")
	}

	log.Infof("⚙️ Setting up SMS client to: %v", opts.MessengerOptions.MessengerType)
	messengerClient, err := message.GetClient(*opts.MessengerOptions)
	if err != nil {
		return nil, fmt.Errorf("creating SMS client: %w", err)
	}

	SetInstance(instanceName, messengerClient)
	return messengerClient, nil
}
