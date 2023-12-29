package events

import (
	"context"
)

const (
	ReceiverWalletSMSInvitationTopic              = "receiver-wallet-sms-invitation"
	PatchAnchorPlatformTransactionCompletionTopic = "patch-anchor-platform-transaction-completion"
)

type EventHandler interface {
	Name() string
	CanHandleMessage(ctx context.Context, message *Message) bool
	Handle(ctx context.Context, message *Message)
}

type EventHandlerOptions struct {
	MaxInvitationSMSResendAttempts int
}
