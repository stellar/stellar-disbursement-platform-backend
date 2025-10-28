package validators

import (
	"slices"
	"strings"

	"github.com/stellar/go/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// CreateDirectPaymentRequest represents the request structure for creating direct payments
type CreateDirectPaymentRequest struct {
	Amount            string                `json:"amount"`
	Asset             DirectPaymentAsset    `json:"asset"`
	Receiver          DirectPaymentReceiver `json:"receiver"`
	Wallet            DirectPaymentWallet   `json:"wallet"`
	ExternalPaymentID *string               `json:"external_payment_id,omitempty"`
}

type DirectPaymentAsset struct {
	ID         *string `json:"id,omitempty"`
	Type       *string `json:"type,omitempty"` // "native", "classic", "contract", "fiat"
	Code       *string `json:"code,omitempty"`
	Issuer     *string `json:"issuer,omitempty"`
	ContractID *string `json:"contract_id,omitempty"`
}

type DirectPaymentReceiver struct {
	ID            *string `json:"id,omitempty"`
	Email         *string `json:"email,omitempty"`
	PhoneNumber   *string `json:"phone_number,omitempty"`
	WalletAddress *string `json:"wallet_address,omitempty"`
}

type DirectPaymentWallet struct {
	ID      *string `json:"id,omitempty"`
	Address *string `json:"address,omitempty"`
}

type DirectPaymentValidator struct {
	*Validator
}

// Asset type constants
const (
	AssetTypeNative   = "native"
	AssetTypeClassic  = "classic"
	AssetTypeContract = "contract"
	AssetTypeFiat     = "fiat"
)

func NewDirectPaymentValidator() *DirectPaymentValidator {
	return &DirectPaymentValidator{Validator: NewValidator()}
}

func (v *DirectPaymentValidator) ValidateCreateDirectPaymentRequest(reqBody *CreateDirectPaymentRequest) *CreateDirectPaymentRequest {
	v.Check(reqBody != nil, "body", "request body is empty")
	if v.HasErrors() {
		return nil
	}

	amount := strings.TrimSpace(reqBody.Amount)
	v.Check(amount != "", "amount", "amount is required")
	if amount != "" {
		v.CheckError(utils.ValidateAmount(amount), "amount", "")
	}

	v.validateAssetReference(&reqBody.Asset)

	v.validateReceiverReference(&reqBody.Receiver)

	v.validateWalletReference(&reqBody.Wallet)

	if v.HasErrors() {
		return nil
	}

	// Return clean request with trimmed fields
	modifiedReq := &CreateDirectPaymentRequest{
		Amount:            amount,
		Asset:             reqBody.Asset,
		Receiver:          reqBody.Receiver,
		Wallet:            reqBody.Wallet,
		ExternalPaymentID: reqBody.ExternalPaymentID,
	}

	return modifiedReq
}

// validateAssetReference validates asset reference structure and required fields
func (v *DirectPaymentValidator) validateAssetReference(asset *DirectPaymentAsset) {
	hasID := asset.ID != nil && strings.TrimSpace(*asset.ID) != ""
	hasType := asset.Type != nil && strings.TrimSpace(*asset.Type) != ""

	// Must specify either id or type
	v.Check(hasID || hasType, "asset", "asset reference is required - must specify either id or type")

	// Cannot specify both
	v.Check(!hasID || !hasType, "asset", "asset reference must specify either id or type, not both")

	// If type is specified, validate it and required fields
	if hasType {
		assetType := strings.TrimSpace(*asset.Type)
		validTypes := []string{AssetTypeNative, AssetTypeClassic, AssetTypeContract, AssetTypeFiat}
		v.Check(slices.Contains(validTypes, assetType), "asset.type",
			"invalid asset type, must be one of: native, classic, contract, fiat")

		if v.HasErrors() {
			return
		}

		switch assetType {
		case AssetTypeClassic:
			v.Check(asset.Code != nil && strings.TrimSpace(*asset.Code) != "",
				"asset.code", "code is required for classic asset")
			v.Check(asset.Issuer != nil && strings.TrimSpace(*asset.Issuer) != "",
				"asset.issuer", "issuer is required for classic asset")

			if asset.Issuer != nil && strings.TrimSpace(*asset.Issuer) != "" {
				issuer := strings.TrimSpace(*asset.Issuer)
				v.Check(strkey.IsValidEd25519PublicKey(issuer),
					"asset.issuer", "invalid stellar account ID format for issuer")
			}

		case AssetTypeContract:
			v.Check(asset.ContractID != nil && strings.TrimSpace(*asset.ContractID) != "",
				"asset.contract_id", "contract_id is required for contract asset")

		case AssetTypeFiat:
			v.Check(asset.Code != nil && strings.TrimSpace(*asset.Code) != "",
				"asset.code", "code is required for fiat asset")
		}

		*asset.Type = assetType
	}

	if asset.Code != nil {
		code := strings.TrimSpace(*asset.Code)
		*asset.Code = code
	}
	if asset.Issuer != nil {
		issuer := strings.TrimSpace(*asset.Issuer)
		*asset.Issuer = issuer
	}
	if asset.ContractID != nil {
		contractID := strings.TrimSpace(*asset.ContractID)
		*asset.ContractID = contractID
	}
}

// validateReceiverReference validates receiver reference structure and formats
func (v *DirectPaymentValidator) validateReceiverReference(receiver *DirectPaymentReceiver) {
	hasID := receiver.ID != nil && strings.TrimSpace(*receiver.ID) != ""
	hasEmail := receiver.Email != nil && strings.TrimSpace(*receiver.Email) != ""
	hasPhone := receiver.PhoneNumber != nil && strings.TrimSpace(*receiver.PhoneNumber) != ""
	hasWallet := receiver.WalletAddress != nil && strings.TrimSpace(*receiver.WalletAddress) != ""

	identifierCount := 0
	if hasID {
		identifierCount++
	}
	if hasEmail {
		identifierCount++
	}
	if hasPhone {
		identifierCount++
	}
	if hasWallet {
		identifierCount++
	}

	v.Check(identifierCount > 0, "receiver",
		"receiver reference is required - must specify id, email, phone_number, or wallet_address")
	v.Check(identifierCount == 1, "receiver",
		"receiver reference must specify exactly one identifier")

	if hasEmail {
		email := strings.TrimSpace(*receiver.Email)
		v.CheckError(utils.ValidateEmail(email), "receiver.email", "")
		*receiver.Email = email
	}

	if hasPhone {
		phone := strings.TrimSpace(*receiver.PhoneNumber)
		v.CheckError(utils.ValidatePhoneNumber(phone), "receiver.phone_number", "")
		*receiver.PhoneNumber = phone
	}

	if hasWallet {
		walletAddr := strings.TrimSpace(*receiver.WalletAddress)
		v.Check(strkey.IsValidEd25519PublicKey(walletAddr) || strkey.IsValidContractAddress(walletAddr),
			"receiver.wallet_address", "invalid stellar address format")
	}

	if hasID {
		id := strings.TrimSpace(*receiver.ID)
		*receiver.ID = id
	}
}

func (v *DirectPaymentValidator) validateWalletReference(wallet *DirectPaymentWallet) {
	hasID := wallet.ID != nil && strings.TrimSpace(*wallet.ID) != ""
	hasAddress := wallet.Address != nil && strings.TrimSpace(*wallet.Address) != ""

	// Must specify either id or address, not both:
	v.Check(hasID || hasAddress, "wallet",
		"wallet reference is required - must specify either id or address")
	v.Check(!hasID || !hasAddress, "wallet",
		"wallet reference must specify either id or address, not both")

	if hasAddress {
		address := strings.TrimSpace(*wallet.Address)
		v.Check(strkey.IsValidEd25519PublicKey(address) || strkey.IsValidContractAddress(address),
			"wallet.address", "invalid stellar address format")
	}

	if hasID {
		id := strings.TrimSpace(*wallet.ID)
		*wallet.ID = id
	}
}
