package schemas

import "time"

type EventReceiverWalletSMSInvitationData struct {
	ReceiverWalletID string `json:"receiver_wallet_id"`
}

type EventPaymentFromSubmitterData struct {
	TransactionID string `json:"transaction_id"`
}

type EventPatchAnchorPlatformTransactionCompletionData struct {
	PaymentID            string    `json:"payment_id"`
	PaymentStatus        string    `json:"payment_status"`
	PaymentStatusMessage string    `json:"payment_status_message"`
	PaymentCompletedAt   time.Time `json:"completed_at"`
	StellarTransactionID string    `json:"stellar_transaction_id"`
}
