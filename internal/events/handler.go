package events

import (
	"context"
)

// Topic Names
const (
	ReceiverWalletNewInvitationTopic = "events.receiver-wallets.new_invitation"
	PaymentCompletedTopic            = "events.payment.payment_completed"
)

// Type Names
const (
	RetryReceiverWalletSMSInvitationType = "retry-receiver-wallet-sms-invitation"
	BatchReceiverWalletSMSInvitationType = "batch-receiver-wallet-sms-invitation"
	PaymentCompletedSuccessType          = "payment-completed-success"
	PaymentCompletedErrorType            = "payment-completed-error"
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
