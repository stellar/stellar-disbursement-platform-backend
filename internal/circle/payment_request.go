package circle

import (
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type APIType string

const (
	APITypeTransfers APIType = "transfers"
	APITypePayout    APIType = "payout"
)

type PaymentRequest struct {
	SourceWalletID            string
	RecipientID               string
	DestinationStellarAddress string
	APIType                   APIType
	Amount                    string
	StellarAssetCode          string
	IdempotencyKey            string
}

// GetCircleAssetCode converts the request's Stellar asset code to a Circle's asset code.
func (p PaymentRequest) GetCircleAssetCode() (string, error) {
	switch p.StellarAssetCode {
	case assets.USDCAssetCode:
		return "USD", nil
	case assets.EURCAssetCode:
		return "EUR", nil
	default:
		return "", fmt.Errorf("unsupported asset code for CIRCLE: %s", p.StellarAssetCode)
	}
}

func (p PaymentRequest) Validate() error {
	if !slices.Contains([]APIType{APITypeTransfers, APITypePayout}, p.APIType) {
		return fmt.Errorf("API type %q is not valid, must be one of %v", p.APIType, []APIType{APITypeTransfers, APITypePayout})
	}

	if p.SourceWalletID == "" {
		return fmt.Errorf("source wallet ID is required")
	}

	if p.APIType == APITypePayout && p.RecipientID == "" {
		return fmt.Errorf("recipient ID is required")
	} else if p.APIType == APITypeTransfers && !strkey.IsValidEd25519PublicKey(p.DestinationStellarAddress) {
		return fmt.Errorf("destination stellar address is not a valid public key")
	}

	if err := utils.ValidateAmount(p.Amount); err != nil {
		return fmt.Errorf("amount is not valid: %w", err)
	}

	if p.StellarAssetCode == "" {
		return fmt.Errorf("stellar asset code is required")
	}

	if err := uuid.Validate(p.IdempotencyKey); err != nil {
		return fmt.Errorf("idempotency key is not valid: %w", err)
	}

	return nil
}
