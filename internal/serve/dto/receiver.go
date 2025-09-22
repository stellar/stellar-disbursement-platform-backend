package dto

import "github.com/stellar/stellar-disbursement-platform-backend/internal/data"

// CreateReceiverRequest represents the request structure for creating receivers
type CreateReceiverRequest struct {
	Email         string                        `json:"email"`
	PhoneNumber   string                        `json:"phone_number"`
	ExternalID    string                        `json:"external_id"`
	Verifications []ReceiverVerificationRequest `json:"verifications"`
	Wallets       []ReceiverWalletRequest       `json:"wallets"`
}

// ReceiverVerificationRequest represents a verification request
type ReceiverVerificationRequest struct {
	Type  data.VerificationType `json:"type"`
	Value string                `json:"value"`
}

// ReceiverWalletRequest represents a wallet request
type ReceiverWalletRequest struct {
	Address string `json:"address"`
	Memo    string `json:"memo,omitempty"`
}
