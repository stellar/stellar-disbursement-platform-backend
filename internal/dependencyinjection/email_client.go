package dependencyinjection

import (
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

const EmailClientInstanceName = "email_client_instance"

type EmailClientOptions struct {
	EmailType        message.MessengerType
	MessengerOptions *message.MessengerOptions
}

// buildEmailClientInstanceName sets up a instance name for the email messenger type
// to either be created and stored, also retrived later, so we can have a instance
// for each type at the same time.
func buildEmailClientInstanceName(emailClientType message.MessengerType) string {
	return fmt.Sprintf("%s-%s", EmailClientInstanceName, string(emailClientType))
}

// NewEmailClient creates a new email client instance, or retrives a instance that
// was already created before.
func NewEmailClient(opts EmailClientOptions) (message.MessengerClient, error) {
	if !opts.EmailType.IsEmail() {
		return nil, fmt.Errorf("trying to create a Email client with a non-supported Email type: %q", opts.EmailType)
	}

	if opts.MessengerOptions == nil {
		opts.MessengerOptions = &message.MessengerOptions{}
	}
	opts.MessengerOptions.MessengerType = opts.EmailType

	// If there is already an instance of the service, we return the same instance
	instanceName := buildEmailClientInstanceName(opts.MessengerOptions.MessengerType)
	if instance, ok := dependenciesStoreMap[instanceName]; ok {
		if emailClientInstance, ok := instance.(message.MessengerClient); ok {
			return emailClientInstance, nil
		}
		return nil, fmt.Errorf("trying to cast pre-existing Email client for depencency injection")
	}

	log.Infof("⚙️ Setting Email client to: %v", opts.MessengerOptions.MessengerType)
	messengerClient, err := message.GetClient(*opts.MessengerOptions)
	if err != nil {
		return nil, fmt.Errorf("creating Email client: %w", err)
	}

	setInstance(instanceName, messengerClient)
	return messengerClient, nil
}
