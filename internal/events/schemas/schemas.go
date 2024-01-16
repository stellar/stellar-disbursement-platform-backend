package schemas

type EventReceiverWalletSMSInvitationData struct {
	ReceiverWalletID string `json:"receiver_wallet_id"`
}

type EventPaymentFromSubmitterData struct {
	TransactionID string `json:"transaction_id"`
}
