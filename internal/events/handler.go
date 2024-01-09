package events

import (
	"context"
)

// Topic Names
const (
	ReceiverWalletSMSInvitationTopic = "receiver-wallet-sms-invitation"
)

// Type Names
const (
	RetryReceiverWalletSMSInvitationType = "retry-receiver-wallet-sms-invitation"
	BatchReceiverWalletSMSInvitationType = "batch-receiver-wallet-sms-invitation"
)

type EventHandler interface {
	Name() string
	CanHandleMessage(ctx context.Context, message *Message) bool
	Handle(ctx context.Context, message *Message)
}

type EventHandlerOptions struct {
	MaxInvitationSMSResendAttempts int
}
