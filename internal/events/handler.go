package events

import (
	"context"
)

// Topic Names
//
// Note: when adding a new topic here, please, add the new topic to `kafka-init` service command on dev/docker-compose-sdp-anchor.yml.
//
//	`kafka-topics.sh --create --if-not-exists --topic events.new-topic ...`
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
