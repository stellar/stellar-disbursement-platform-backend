package schemas

import "time"

type EventReceiverWalletInvitationData struct {
	ReceiverWalletID string `json:"receiver_wallet_id"`
}

type EventPaymentCompletedData struct {
	TransactionID        string    `json:"transaction_id"`
	PaymentID            string    `json:"payment_id"`
	PaymentStatus        string    `json:"payment_status"`
	PaymentStatusMessage string    `json:"payment_status_message"`
	PaymentCompletedAt   time.Time `json:"completed_at"`
	StellarTransactionID string    `json:"stellar_transaction_id"`
}

type PaymentReadyToPay struct {
	ID string `json:"id"`
}

type EventPaymentsReadyToPayData struct {
	TenantID string              `json:"tenant_id"`
	Payments []PaymentReadyToPay `json:"payments"`
}

type EventWalletCreationCompletedData struct {
	TransactionID               string    `json:"transaction_id"`
	WalletCreationID            string    `json:"wallet_creation_id"`
	WalletCreationStatus        string    `json:"wallet_creation_status"`
	WalletCreationStatusMessage string    `json:"wallet_creation_status_message"`
	WalletCreationCompletedAt   time.Time `json:"completed_at"`
	StellarTransactionID        string    `json:"stellar_transaction_id"`
}

type EventSponsoredTransactionCompletedData struct {
	TransactionID                     string    `json:"transaction_id"`
	SponsoredTransactionID            string    `json:"sponsored_transaction_id"`
	SponsoredTransactionStatus        string    `json:"sponsored_transaction_status"`
	SponsoredTransactionStatusMessage string    `json:"sponsored_transaction_status_message"`
	SponsoredTransactionCompletedAt   time.Time `json:"completed_at"`
	StellarTransactionID              string    `json:"stellar_transaction_id"`
}
