package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/message"
)

const MessageDispatcherInstanceName = "message_dispatcher_instance"

type MessageDispatcherOpts struct {
	EmailOpts *EmailClientOptions
	SMSOpts   *SMSClientOptions
}

func NewMessageDispatcher(ctx context.Context, opts MessageDispatcherOpts) (*message.MessageDispatcher, error) {
	if instance, ok := GetInstance(MessageDispatcherInstanceName); ok {
		if dispatcherInstance, ok := instance.(*message.MessageDispatcher); ok {
			return dispatcherInstance, nil
		}
		return nil, fmt.Errorf("trying to cast pre-existing MessageDispatcher for dependency injection")
	}

	dispatcher := message.NewMessageDispatcher()

	if opts.EmailOpts != nil {
		emailClient, err := NewEmailClient(*opts.EmailOpts)
		if err != nil {
			return nil, fmt.Errorf("creating email client: %w", err)
		}
		dispatcher.RegisterClient(ctx, message.MessageChannelEmail, emailClient)
	}

	if opts.SMSOpts != nil {
		smsClient, err := NewSMSClient(*opts.SMSOpts)
		if err != nil {
			return nil, fmt.Errorf("creating SMS client: %w", err)
		}
		dispatcher.RegisterClient(ctx, message.MessageChannelSMS, smsClient)
	}

	SetInstance(MessageDispatcherInstanceName, dispatcher)
	return dispatcher, nil
}
