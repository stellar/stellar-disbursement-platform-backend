package circle

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type PaymentRequest struct {
	SourceWalletID            string
	DestinationStellarAddress string
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
	if p.SourceWalletID == "" {
		return fmt.Errorf("source wallet ID is required")
	}

	if !strkey.IsValidEd25519PublicKey(p.DestinationStellarAddress) {
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
