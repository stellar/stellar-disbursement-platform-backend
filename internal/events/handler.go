package events

import (
	"context"
)

// Topic Names
const (
	ReceiverWalletNewInvitationTopic              = "events.receiver-wallets.new-invitation"
	PaymentFromSubmitterTopic                     = "events.transaction-submitter.payment-from-submitter"
	PatchAnchorPlatformTransactionCompletionTopic = "events.transaction-submitter.transaction-completion"
)

// Type Names
const (
	RetryReceiverWalletSMSInvitationType                = "retry-receiver-wallet-sms-invitation"
	BatchReceiverWalletSMSInvitationType                = "batch-receiver-wallet-sms-invitation"
	SyncSuccessPaymentFromSubmitterType                 = "sync-success-payment-from-submitter"
	SyncErrorPaymentFromSubmitterType                   = "sync-error-payment-from-submitter"
	PatchSuccessAnchorPlatformTransactionCompletionType = "patch-success-anchor-platform-transaction-completion"
	PatchFailedAnchorPlatformTransactionCompletionType  = "patch-failed-anchor-platform-transaction-completion"
)

type EventHandler interface {
	Name() string
	CanHandleMessage(ctx context.Context, message *Message) bool
	Handle(ctx context.Context, message *Message)
}

type EventHandlerOptions struct {
	MaxInvitationSMSResendAttempts int
}
