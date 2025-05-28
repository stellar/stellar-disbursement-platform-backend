package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/stellar/go/strkey"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
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
		// Validate asset type
		validTypes := []string{"native", "classic", "contract", "fiat"}
		found := false
		for _, validType := range validTypes {
			if *ref.Type == validType {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid asset type: %s", *ref.Type)
		}

		switch *ref.Type {
		case "classic":
			if ref.Code == nil || strings.TrimSpace(*ref.Code) == "" {
				return fmt.Errorf("code is required for classic asset")
			}
			if ref.Issuer == nil || strings.TrimSpace(*ref.Issuer) == "" {
				return fmt.Errorf("issuer is required for classic asset")
			}
		case "contract":
			if ref.ContractID == nil || strings.TrimSpace(*ref.ContractID) == "" {
				return fmt.Errorf("contract_id is required for contract asset")
			}
		case "fiat":
			if ref.Code == nil || strings.TrimSpace(*ref.Code) == "" {
				return fmt.Errorf("code is required for fiat asset")
			}
		}
	}

	if referenceCount == 0 {
		return fmt.Errorf("asset must be specified by id or type")
	}

	if referenceCount > 1 {
		return fmt.Errorf("asset must be specified by either id or type, not both")
	}

	return nil
}

func (ar *AssetResolver) Resolve(ctx context.Context, _ db.SQLExecuter, ref AssetReference) (*data.Asset, error) {
	if err := ar.Validate(ref); err != nil {
		return nil, fmt.Errorf("validating asset reference: %w", err)
	}

	if ref.ID != nil {
		return ar.models.Assets.Get(ctx, *ref.ID)
	}

	if ref.Type != nil {
		switch *ref.Type {
		case "native":
			return ar.models.Assets.GetByCodeAndIssuer(ctx, "XLM", "")
		case "classic":
			return ar.models.Assets.GetByCodeAndIssuer(ctx, *ref.Code, *ref.Issuer)
		case "contract":
			// TODO: Implement contract asset resolution
			return nil, fmt.Errorf("contract assets not yet supported")
		case "fiat":
			// TODO: Implement fiat asset resolution
			return nil, fmt.Errorf("fiat assets not yet supported")
		default:
			return nil, fmt.Errorf("unsupported asset type: %s", *ref.Type)
		}
	}

	return nil, fmt.Errorf("asset must be specified by id or type")
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
			return fmt.Errorf("invalid email: %w", err)
		}
	}
	if ref.PhoneNumber != nil && strings.TrimSpace(*ref.PhoneNumber) != "" {
		referenceCount++
		if err := utils.ValidatePhoneNumber(*ref.PhoneNumber); err != nil {
			return fmt.Errorf("invalid phone number: %w", err)
		}
	}
	if ref.WalletAddress != nil && strings.TrimSpace(*ref.WalletAddress) != "" {
		referenceCount++
		if !strkey.IsValidEd25519PublicKey(*ref.WalletAddress) {
			return fmt.Errorf("invalid stellar wallet address format: %s", *ref.WalletAddress)
		}
	}

	if referenceCount == 0 {
		return fmt.Errorf("receiver must be specified by id, email, phone_number, or wallet_address")
	}

	if referenceCount > 1 {
		return fmt.Errorf("receiver must be specified by only one identifier")
	}

	return nil
}

func (r *ReceiverResolver) Resolve(ctx context.Context, sqlExec db.SQLExecuter, ref ReceiverReference) (*data.Receiver, error) {
	if err := r.Validate(ref); err != nil {
		return nil, fmt.Errorf("validating receiver reference: %w", err)
	}

	if ref.ID != nil {
		return r.models.Receiver.Get(ctx, sqlExec, *ref.ID)
	}

	contactInfo := ref.GetContactInfo()
	if contactInfo != "" {
		receivers, err := r.models.Receiver.GetByContacts(ctx, sqlExec, contactInfo)
		if err != nil {
			return nil, fmt.Errorf("getting receiver by contact: %w", err)
		}
		if len(receivers) == 0 {
			return nil, fmt.Errorf("no receiver found with contact info")
		}
		if len(receivers) > 1 {
			return nil, fmt.Errorf("multiple receivers found with contact info")
		}
		return receivers[0], nil
	}

	if ref.WalletAddress != nil {
		receiver, err := r.models.Receiver.GetByWalletAddress(ctx, sqlExec, *ref.WalletAddress)
		if err != nil {
			if errors.Is(err, data.ErrRecordNotFound) {
				return nil, fmt.Errorf("no receiver found with wallet address %s", *ref.WalletAddress)
			}
			return nil, fmt.Errorf("getting receiver by wallet address: %w", err)
		}
		return receiver, nil
	}

	return nil, fmt.Errorf("receiver must be specified by id, email, or phone number")
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
		if !strkey.IsValidEd25519PublicKey(*ref.Address) {
			return fmt.Errorf("invalid stellar address format")
		}
	}

	if referenceCount == 0 {
		return fmt.Errorf("wallet must be specified by id or address")
	}

	if referenceCount > 1 {
		return fmt.Errorf("wallet must be specified by either id or address, not both")
	}

	return nil
}

func (r *WalletResolver) Resolve(ctx context.Context, dbTx db.SQLExecuter, ref WalletReference) (*data.Wallet, error) {
	if err := r.Validate(ref); err != nil {
		return nil, fmt.Errorf("validating wallet reference: %w", err)
	}

	if ref.ID != nil {
		return r.models.Wallets.Get(ctx, *ref.ID)
	}

	if ref.Address != nil {

		// Find the UserManagedWallet
		wallets, err := r.models.Wallets.FindWallets(ctx,
			data.NewFilter(data.FilterUserManaged, true),
			data.NewFilter(data.FilterEnabledWallets, true))
		if err != nil {
			return nil, fmt.Errorf("finding user managed wallets: %w", err)
		}

		// Since user-managed wallets don't store addresses in the wallet table,
		// we return the user-managed wallet and the address will be used
		// when creating/updating the receiver_wallet record
		if len(wallets) == 0 {
			return nil, fmt.Errorf("no user managed wallet found")
		}

		return &wallets[0], nil
	}

	return nil, fmt.Errorf("wallet must be specified by id or address")
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
