package circle

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type APIType string

const (
	APITypePayouts   APIType = "PAYOUTS"
	APITypeTransfers APIType = "TRANSFERS"
)

func AllAPITypes() []APIType {
	return []APIType{APITypePayouts, APITypeTransfers}
}

func ParseAPIType(messengerTypeStr string) (APIType, error) {
	messageTypeStrUpper := strings.ToUpper(messengerTypeStr)
	mType := APIType(messageTypeStrUpper)

	if slices.Contains(AllAPITypes(), mType) {
		return mType, nil
	}

	return "", fmt.Errorf("invalid Circle API type %q, must be one of %v", messageTypeStrUpper, AllAPITypes())
}

type PaymentRequest struct {
	SourceWalletID            string
	RecipientID               string
	DestinationStellarAddress string
	DestinationStellarMemo    string
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
	if _, err := ParseAPIType(string(p.APIType)); err != nil {
		return err
	}

	if p.SourceWalletID == "" {
		return fmt.Errorf("source wallet ID is required")
	}

	if p.APIType == APITypePayouts && p.RecipientID == "" {
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
