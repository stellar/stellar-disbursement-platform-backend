package events

import (
	"context"
)

// Topic Names
//
// Note: when adding a new topic here, please, add the new topic to `kafka-init` service command on dev/docker-compose-sdp.yml.
//
//	`kafka-topics.sh --create --if-not-exists --topic events.new-topic ...`
const (
	ReceiverWalletNewInvitationTopic = "events.receiver-wallets.new_invitation"
	PaymentCompletedTopic            = "events.payment.payment_completed"
	PaymentReadyToPayTopic           = "events.payment.ready_to_pay"
	CirclePaymentReadyToPayTopic     = "events.payment.circle_ready_to_pay"
)

// Type Names
const (
	RetryReceiverWalletInvitationType              = "retry-receiver-wallet-sms-invitation"
	BatchReceiverWalletInvitationType              = "batch-receiver-wallet-sms-invitation"
	PaymentCompletedSuccessType                    = "payment-completed-success"
	PaymentCompletedErrorType                      = "payment-completed-error"
	PaymentReadyToPayDisbursementStarted           = "payment-ready-to-pay-disbursement-started"
	PaymentReadyToPayReceiverVerificationCompleted = "payment-ready-to-pay-receiver-verification-completed"
	PaymentReadyToPayRetryFailedPayment            = "payment-ready-to-pay-retry-failed-payment"
	PaymentReadyToPayDirectPayment                 = "payment-direct-payment"
)

type EventHandler interface {
	Name() string
	CanHandleMessage(ctx context.Context, message *Message) bool
	Handle(ctx context.Context, message *Message) error
}
