package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/stellar/go-stellar-sdk/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// Entity type constants
const (
	EntityTypeAsset    = "asset"
	EntityTypeReceiver = "receiver"
	EntityTypeWallet   = "wallet"
)

// Field name constants
const (
	FieldReference     = "reference"
	FieldType          = "type"
	FieldCode          = "code"
	FieldIssuer        = "issuer"
	FieldContractID    = "contract_id"
	FieldEmail         = "email"
	FieldPhoneNumber   = "phone_number"
	FieldWalletAddress = "wallet_address"
	FieldAddress       = "address"
)

// Asset type constants
const (
	AssetTypeNative   = "native"
	AssetTypeClassic  = "classic"
	AssetTypeContract = "contract"
	AssetTypeFiat     = "fiat"
)

type EntityResolver[T any, R any] interface {
	Resolve(ctx context.Context, sqlExec db.SQLExecuter, ref R) (*T, error)
	Validate(ref R) error
}

type ResolverFactory struct {
	assetResolver    *AssetResolver
	receiverResolver *ReceiverResolver
	walletResolver   *WalletResolver
}

func NewResolverFactory(models *data.Models) *ResolverFactory {
	return &ResolverFactory{
		assetResolver:    NewAssetResolver(models),
		receiverResolver: NewReceiverResolver(models),
		walletResolver:   NewWalletResolver(models),
	}
}

func (rf *ResolverFactory) Asset() *AssetResolver {
	return rf.assetResolver
}

func (rf *ResolverFactory) Receiver() *ReceiverResolver {
	return rf.receiverResolver
}

func (rf *ResolverFactory) Wallet() *WalletResolver {
	return rf.walletResolver
}

type ValidationError struct {
	EntityType string
	Field      string
	Message    string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error for %s.%s: %s", e.EntityType, e.Field, e.Message)
}

type NotFoundError struct {
	EntityType string
	Reference  string
	Message    string
}

func (e NotFoundError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s not found: %s", e.EntityType, e.Message)
	}
	return fmt.Sprintf("%s not found with reference: %s", e.EntityType, e.Reference)
}

type UnsupportedError struct {
	EntityType string
	Feature    string
}

func (e UnsupportedError) Error() string {
	return fmt.Sprintf("%s: %s not yet supported", e.EntityType, e.Feature)
}

type AmbiguousReferenceError struct {
	EntityType string
	Reference  string
	Count      int
}

func (e AmbiguousReferenceError) Error() string {
	return fmt.Sprintf("ambiguous %s reference %s: found %d matches", e.EntityType, e.Reference, e.Count)
}

type AssetReference struct {
	ID         *string `json:"id,omitempty"`
	Type       *string `json:"type,omitempty"` // "native", "classic", "contract", "fiat"
	Code       *string `json:"code,omitempty"`
	Issuer     *string `json:"issuer,omitempty"`
	ContractID *string `json:"contract_id,omitempty"` // For future implementation
}

type ReceiverReference struct {
	ID            *string `json:"id,omitempty"`
	Email         *string `json:"email,omitempty"`
	PhoneNumber   *string `json:"phone_number,omitempty"`
	WalletAddress *string `json:"wallet_address,omitempty"`
}

type WalletReference struct {
	ID      *string `json:"id,omitempty"`
	Address *string `json:"address,omitempty"`
}

type AssetResolver struct {
	models *data.Models
}

func NewAssetResolver(models *data.Models) *AssetResolver {
	return &AssetResolver{models: models}
}

func (ar *AssetResolver) Validate(ref AssetReference) error {
	referenceCount := 0

	if ref.ID != nil && strings.TrimSpace(*ref.ID) != "" {
		referenceCount++
	}

	if ref.Type != nil && strings.TrimSpace(*ref.Type) != "" {
		referenceCount++
		validTypes := []string{AssetTypeNative, AssetTypeClassic, AssetTypeContract, AssetTypeFiat}
		found := slices.Contains(validTypes, *ref.Type)
		if !found {
			return ValidationError{
				EntityType: "asset",
				Field:      "type",
				Message:    fmt.Sprintf("invalid type '%s', must be one of: %s", *ref.Type, strings.Join(validTypes, ", ")),
			}
		}

		switch *ref.Type {
		case AssetTypeClassic:
			if ref.Code == nil || strings.TrimSpace(*ref.Code) == "" {
				return ValidationError{
					EntityType: EntityTypeAsset,
					Field:      FieldCode,
					Message:    "required for classic asset",
				}
			}

			if ref.ContractID != nil {
				return ValidationError{
					EntityType: EntityTypeAsset,
					Field:      FieldContractID,
					Message:    "field not supported for classic asset",
				}
			}
			if ref.Issuer == nil || strings.TrimSpace(*ref.Issuer) == "" {
				return ValidationError{
					EntityType: EntityTypeAsset,
					Field:      FieldIssuer,
					Message:    "required for classic asset",
				}
			}
		case AssetTypeContract:
			if ref.ContractID == nil || strings.TrimSpace(*ref.ContractID) == "" {
				return ValidationError{
					EntityType: EntityTypeAsset,
					Field:      FieldContractID,
					Message:    "required for contract asset",
				}
			}
			if !strkey.IsValidContractAddress(*ref.ContractID) {
				return ValidationError{
					EntityType: EntityTypeAsset,
					Field:      FieldContractID,
					Message:    "invalid contract format provided",
				}
			}
		case AssetTypeFiat:
			if ref.Code == nil || strings.TrimSpace(*ref.Code) == "" {
				return ValidationError{
					EntityType: EntityTypeAsset,
					Field:      FieldCode,
					Message:    "required for fiat asset",
				}
			}
		}
	}

	if referenceCount == 0 {
		return ValidationError{
			EntityType: EntityTypeAsset,
			Field:      FieldReference,
			Message:    "must be specified by id or type",
		}
	}

	if referenceCount > 1 {
		return ValidationError{
			EntityType: EntityTypeAsset,
			Field:      FieldReference,
			Message:    "must be specified by either id or type, not both",
		}
	}

	return nil
}

func (ar *AssetResolver) Resolve(ctx context.Context, _ db.SQLExecuter, ref AssetReference) (*data.Asset, error) {
	if err := ar.Validate(ref); err != nil {
		return nil, err
	}

	if ref.ID != nil {
		asset, err := ar.models.Assets.Get(ctx, *ref.ID)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return nil, NotFoundError{
					EntityType: EntityTypeAsset,
					Reference:  *ref.ID,
				}
			}
			return nil, fmt.Errorf("getting asset by ID: %w", err)
		}
		return asset, nil
	}

	if ref.Type != nil {
		switch *ref.Type {
		case AssetTypeNative:
			asset, err := ar.models.Assets.GetByCodeAndIssuer(ctx, "XLM", "")
			if err != nil {
				if errors.Is(err, data.ErrRecordNotFound) {
					return nil, NotFoundError{
						EntityType: EntityTypeAsset,
						Message:    "native XLM asset not found",
					}
				}
				return nil, fmt.Errorf("getting native asset: %w", err)
			}
			return asset, nil
		case AssetTypeClassic:
			asset, err := ar.models.Assets.GetByCodeAndIssuer(ctx, *ref.Code, *ref.Issuer)
			if err != nil {
				if errors.Is(err, data.ErrRecordNotFound) {
					return nil, NotFoundError{
						EntityType: EntityTypeAsset,
						Reference:  fmt.Sprintf("%s:%s", *ref.Code, *ref.Issuer),
					}
				}
				return nil, fmt.Errorf("getting classic asset: %w", err)
			}
			return asset, nil
		case AssetTypeContract:
			return nil, UnsupportedError{
				EntityType: EntityTypeAsset,
				Feature:    "contract assets",
			}
		case AssetTypeFiat:
			return nil, UnsupportedError{
				EntityType: EntityTypeAsset,
				Feature:    "fiat assets",
			}
		}
	}

	return nil, ValidationError{
		EntityType: EntityTypeAsset,
		Field:      FieldReference,
		Message:    "must be specified by id or type",
	}
}

var _ EntityResolver[data.Asset, AssetReference] = (*AssetResolver)(nil)

type ReceiverResolver struct {
	models *data.Models
}

func NewReceiverResolver(models *data.Models) *ReceiverResolver {
	return &ReceiverResolver{models: models}
}

func (r *ReceiverResolver) Validate(ref ReceiverReference) error {
	referenceCount := 0

	if ref.ID != nil && strings.TrimSpace(*ref.ID) != "" {
		referenceCount++
	}
	if ref.Email != nil && strings.TrimSpace(*ref.Email) != "" {
		referenceCount++
		if err := utils.ValidateEmail(*ref.Email); err != nil {
			return ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldEmail,
				Message:    fmt.Sprintf("invalid format: %s", *ref.Email),
			}
		}
	}
	if ref.PhoneNumber != nil && strings.TrimSpace(*ref.PhoneNumber) != "" {
		referenceCount++
		if err := utils.ValidatePhoneNumber(*ref.PhoneNumber); err != nil {
			return ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldPhoneNumber,
				Message:    fmt.Sprintf("invalid format: %s", *ref.PhoneNumber),
			}
		}
	}
	if ref.WalletAddress != nil && strings.TrimSpace(*ref.WalletAddress) != "" {
		referenceCount++
		if !strkey.IsValidEd25519PublicKey(*ref.WalletAddress) && !strkey.IsValidContractAddress(*ref.WalletAddress) {
			return ValidationError{
				EntityType: EntityTypeReceiver,
				Field:      FieldWalletAddress,
				Message:    fmt.Sprintf("invalid stellar address format: %s", *ref.WalletAddress),
			}
		}
	}

	if referenceCount == 0 {
		return ValidationError{
			EntityType: EntityTypeReceiver,
			Field:      FieldReference,
			Message:    "must be specified by id, email, phone_number, or wallet_address",
		}
	}

	if referenceCount > 1 {
		return ValidationError{
			EntityType: EntityTypeReceiver,
			Field:      FieldReference,
			Message:    "must be specified by only one identifier",
		}
	}

	return nil
}

func (r *ReceiverResolver) Resolve(ctx context.Context, sqlExec db.SQLExecuter, ref ReceiverReference) (*data.Receiver, error) {
	if err := r.Validate(ref); err != nil {
		return nil, err
	}

	if ref.ID != nil {
		receiver, err := r.models.Receiver.Get(ctx, sqlExec, *ref.ID)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return nil, NotFoundError{
					EntityType: EntityTypeReceiver,
					Reference:  *ref.ID,
				}
			}
			return nil, fmt.Errorf("getting receiver by ID: %w", err)
		}
		return receiver, nil
	}

	contactInfo := ref.GetContactInfo()
	if contactInfo != "" {
		receivers, err := r.models.Receiver.GetByContacts(ctx, sqlExec, contactInfo)
		if err != nil {
			return nil, fmt.Errorf("getting receiver by contact: %w", err)
		}
		if len(receivers) == 0 {
			return nil, NotFoundError{
				EntityType: EntityTypeReceiver,
				Message:    fmt.Sprintf("no receiver found with contact info: %s", contactInfo),
			}
		}
		if len(receivers) > 1 {
			return nil, AmbiguousReferenceError{
				EntityType: EntityTypeReceiver,
				Reference:  contactInfo,
				Count:      len(receivers),
			}
		}
		return receivers[0], nil
	}

	if ref.WalletAddress != nil {
		receiver, err := r.models.Receiver.GetByWalletAddress(ctx, sqlExec, *ref.WalletAddress)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return nil, NotFoundError{
					EntityType: EntityTypeReceiver,
					Message:    fmt.Sprintf("no receiver found with wallet address: %s", *ref.WalletAddress),
				}
			}
			return nil, fmt.Errorf("getting receiver by wallet address: %w", err)
		}
		return receiver, nil
	}

	return nil, ValidationError{
		EntityType: EntityTypeReceiver,
		Field:      FieldReference,
		Message:    "must be specified by id, email, phone_number, or wallet_address",
	}
}

var _ EntityResolver[data.Receiver, ReceiverReference] = (*ReceiverResolver)(nil)

type WalletResolver struct {
	models *data.Models
}

func NewWalletResolver(models *data.Models) *WalletResolver {
	return &WalletResolver{models: models}
}

func (r *WalletResolver) Validate(ref WalletReference) error {
	referenceCount := 0

	if ref.ID != nil && strings.TrimSpace(*ref.ID) != "" {
		referenceCount++
	}
	if ref.Address != nil && strings.TrimSpace(*ref.Address) != "" {
		referenceCount++
		if !strkey.IsValidEd25519PublicKey(*ref.Address) && !strkey.IsValidContractAddress(*ref.Address) {
			return ValidationError{
				EntityType: EntityTypeWallet,
				Field:      FieldAddress,
				Message:    fmt.Sprintf("invalid stellar address format: %s", *ref.Address),
			}
		}
	}

	if referenceCount == 0 {
		return ValidationError{
			EntityType: EntityTypeWallet,
			Field:      FieldReference,
			Message:    "must be specified by id or address",
		}
	}

	if referenceCount > 1 {
		return ValidationError{
			EntityType: EntityTypeWallet,
			Field:      FieldReference,
			Message:    "must be specified by either id or address, not both",
		}
	}

	return nil
}

func (r *WalletResolver) Resolve(ctx context.Context, dbTx db.SQLExecuter, ref WalletReference) (*data.Wallet, error) {
	if err := r.Validate(ref); err != nil {
		return nil, err
	}

	if ref.ID != nil {
		wallet, err := r.models.Wallets.Get(ctx, *ref.ID)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return nil, NotFoundError{
					EntityType: EntityTypeWallet,
					Reference:  *ref.ID,
				}
			}
			return nil, fmt.Errorf("getting wallet by ID: %w", err)
		}
		return wallet, nil
	}

	if ref.Address != nil {
		receiverWallet, err := r.models.ReceiverWallet.GetByStellarAccountAndMemo(ctx, *ref.Address, "", nil)
		if err != nil {
			return nil, fmt.Errorf("finding receiver wallet: %w", err)
		}

		if !receiverWallet.Wallet.UserManaged || !receiverWallet.Wallet.Enabled {
			return nil, NotFoundError{
				EntityType: EntityTypeWallet,
				Message:    "no user managed wallet found",
			}
		}

		return &receiverWallet.Wallet, nil
	}

	return nil, ValidationError{
		EntityType: EntityTypeWallet,
		Field:      FieldReference,
		Message:    "must be specified by id or address",
	}
}

var _ EntityResolver[data.Wallet, WalletReference] = (*WalletResolver)(nil)

func (r ReceiverReference) GetContactInfo() string {
	if r.Email != nil && *r.Email != "" {
		return *r.Email
	}
	if r.PhoneNumber != nil && *r.PhoneNumber != "" {
		return *r.PhoneNumber
	}
	return ""
}
