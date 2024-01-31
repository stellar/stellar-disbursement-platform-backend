package events

import (
	"context"
)

// Topic Names
const (
	ReceiverWalletNewInvitationTopic = "events.receiver-wallets.new_invitation"
	PaymentCompletedTopic            = "events.payment.payment_completed"
	PaymentReadyToPayTopic           = "events.payment.ready_to_pay"
)

// Type Names
const (
	RetryReceiverWalletSMSInvitationType           = "retry-receiver-wallet-sms-invitation"
	BatchReceiverWalletSMSInvitationType           = "batch-receiver-wallet-sms-invitation"
	PaymentCompletedSuccessType                    = "payment-completed-success"
	PaymentCompletedErrorType                      = "payment-completed-error"
	PaymentReadyToPayDisbursementStarted           = "payment-ready-to-pay-disbursement-started"
	PaymentReadyToPayReceiverVerificationCompleted = "payment-ready-to-pay-receiver-verification-completed"
	PaymentReadyToPayRetryFailedPayment            = "payment-ready-to-pay-retry-failed-payment"
)

type EventHandler interface {
	Name() string
	CanHandleMessage(ctx context.Context, message *Message) bool
	Handle(ctx context.Context, message *Message)
}

type EventHandlerOptions struct {
	MaxInvitationSMSResendAttempts int
}
