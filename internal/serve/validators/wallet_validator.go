package validators

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/assets"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type AssetReferenceType string

const (
	AssetReferenceTypeID       AssetReferenceType = "id"
	AssetReferenceTypeClassic  AssetReferenceType = "classic"
	AssetReferenceTypeNative   AssetReferenceType = "native"
	AssetReferenceTypeContract AssetReferenceType = "contract"
	AssetReferenceTypeFiat     AssetReferenceType = "fiat"
)

type AssetReference struct {
	// For ID-based reference
	ID string `json:"id,omitempty"`

	Type       string `json:"type,omitempty"`
	Code       string `json:"code,omitempty"`
	Issuer     string `json:"issuer,omitempty"`
	ContractID string `json:"contract_id,omitempty"`
}

type WalletRequest struct {
	Name              string           `json:"name"`
	Homepage          string           `json:"homepage"`
	DeepLinkSchema    string           `json:"deep_link_schema"`
	SEP10ClientDomain string           `json:"sep_10_client_domain"`
	Enabled           *bool            `json:"enabled,omitempty"`
	Assets            []AssetReference `json:"assets,omitempty"`
	AssetsIDs         []string         `json:"assets_ids,omitempty"` // Legacy support
}

type PatchWalletRequest struct {
	Name              *string           `json:"name,omitempty"`
	Homepage          *string           `json:"homepage,omitempty"`
	DeepLinkSchema    *string           `json:"deep_link_schema,omitempty"`
	SEP10ClientDomain *string           `json:"sep_10_client_domain,omitempty"`
	Enabled           *bool             `json:"enabled,omitempty"`
	Assets            *[]AssetReference `json:"assets,omitempty"`
}

type WalletValidator struct {
	*Validator
}

var (
	ErrMemoNotSupportedForContract = errors.New("wallet address memo is not supported for contract addresses")
)

// ValidateWalletAddressMemo asserts whether the supplied memo can be used with
// the given wallet address, and returns the detected memo type if memo is valid.
func ValidateWalletAddressMemo(walletAddress, memo string) (schema.MemoType, error) {
	if memo == "" {
		return "", nil
	}

	switch {
	case strkey.IsValidContractAddress(walletAddress):
		return "", ErrMemoNotSupportedForContract
	case strkey.IsValidEd25519PublicKey(walletAddress):
		_, memoType, err := schema.ParseMemo(memo)
		if err != nil {
			return "", fmt.Errorf("parsing memo %s: %w", memo, err)
		}
		return memoType, nil
	default:
		return "", nil
	}
}

func NewWalletValidator() *WalletValidator {
	return &WalletValidator{Validator: NewValidator()}
}

func (wv *WalletValidator) ValidateCreateWalletRequest(ctx context.Context, reqBody *WalletRequest, enforceHTTPS bool) *WalletRequest {
	// empty body validation
	wv.Check(reqBody != nil, "body", "request body is empty")
	if wv.HasErrors() {
		return nil
	}

	// empty fields validation
	name := strings.TrimSpace(reqBody.Name)
	homepage := strings.TrimSpace(reqBody.Homepage)
	deepLinkSchema := strings.TrimSpace(reqBody.DeepLinkSchema)
	sep10ClientDomain := strings.TrimSpace(reqBody.SEP10ClientDomain)

	wv.Check(name != "", "name", "name is required")
	wv.Check(homepage != "", "homepage", "homepage is required")
	wv.Check(deepLinkSchema != "", "deep_link_schema", "deep_link_schema is required")
	wv.Check(sep10ClientDomain != "", "sep_10_client_domain", "sep_10_client_domain is required")
	wv.Check(len(reqBody.AssetsIDs) != 0 || len(reqBody.Assets) != 0, "assets", "provide at least one 'assets_ids' or 'assets'")
	wv.Check(len(reqBody.AssetsIDs) == 0 || len(reqBody.Assets) == 0, "assets", "cannot use both 'assets_ids' and 'assets' fields simultaneously")
	var processedAssets []AssetReference
	if len(reqBody.Assets) != 0 {
		for i, asset := range reqBody.Assets {
			inferredAsset := wv.inferAssetType(asset)
			if err := inferredAsset.Validate(); err != nil {
				wv.Check(false, fmt.Sprintf("assets[%d]", i), err.Error())
				continue
			}
			processedAssets = append(processedAssets, inferredAsset)
		}
	}

	if wv.HasErrors() {
		return nil
	}

	// fields format validation
	schemes := []string{"https"}
	if !enforceHTTPS {
		schemes = append(schemes, "http")
	}
	wv.CheckError(utils.ValidateURLScheme(homepage, schemes...), "homepage", "")

	deepLinkSchemaURL, err := url.ParseRequestURI(deepLinkSchema)
	if err != nil {
		log.Ctx(ctx).Errorf("parsing deep link schema: %v", err)
		wv.Check(false, "deep_link_schema", "invalid deep link schema provided")
	}

	sep10URL, err := url.Parse(sep10ClientDomain)
	if err != nil {
		log.Ctx(ctx).Errorf("parsing SEP-10 client domain URL: %v", err)
		wv.Check(false, "sep_10_client_domain", "invalid SEP-10 client domain URL provided")
	}

	sep10Host := sep10URL.Host
	if sep10Host == "" {
		sep10Host = sep10URL.String()
	}
	if err := utils.ValidateDNS(sep10Host); err != nil {
		log.Ctx(ctx).Errorf("validating SEP-10 client domain: %v", err)
		wv.Check(false, "sep_10_client_domain", "invalid SEP-10 client domain provided")
	}

	if reqBody.Enabled == nil {
		defEnabled := true
		reqBody.Enabled = &defEnabled
	}

	if wv.HasErrors() {
		return nil
	}

	modifiedReq := &WalletRequest{
		Name:              name,
		Homepage:          homepage,
		DeepLinkSchema:    deepLinkSchemaURL.String(),
		SEP10ClientDomain: sep10Host,
		Assets:            processedAssets,
		AssetsIDs:         reqBody.AssetsIDs,
		Enabled:           reqBody.Enabled,
	}

	return modifiedReq
}

func (wv *WalletValidator) ValidatePatchWalletRequest(ctx context.Context, reqBody *PatchWalletRequest, enforceHTTPS bool) *PatchWalletRequest {
	wv.Check(reqBody != nil, "body", "request body is empty")
	if wv.HasErrors() {
		return nil
	}

	hasField := reqBody.Name != nil || reqBody.Homepage != nil ||
		reqBody.DeepLinkSchema != nil || reqBody.SEP10ClientDomain != nil ||
		reqBody.Enabled != nil || reqBody.Assets != nil

	wv.Check(hasField, "body", "at least one field must be provided for update")
	if wv.HasErrors() {
		return nil
	}

	modifiedReq := &PatchWalletRequest{
		Enabled: reqBody.Enabled,
	}

	// Validate and process name
	if reqBody.Name != nil {
		name := strings.TrimSpace(*reqBody.Name)
		wv.Check(name != "", "name", "name cannot be empty")
		modifiedReq.Name = &name
	}

	// Validate and process homepage
	if reqBody.Homepage != nil {
		homepage := strings.TrimSpace(*reqBody.Homepage)
		wv.Check(homepage != "", "homepage", "homepage cannot be empty")

		homepageURL, err := url.ParseRequestURI(homepage)
		if err != nil {
			log.Ctx(ctx).Errorf("parsing homepage URL: %v", err)
			wv.Check(false, "homepage", "invalid URL format")
		} else {
			schemes := []string{"https"}
			if !enforceHTTPS {
				schemes = append(schemes, "http")
			}
			wv.CheckError(utils.ValidateURLScheme(homepage, schemes...), "homepage", "")
			if !wv.HasErrors() {
				validatedHomepage := homepageURL.String()
				modifiedReq.Homepage = &validatedHomepage
			}
		}
	}

	if reqBody.DeepLinkSchema != nil {
		deepLinkSchema := strings.TrimSpace(*reqBody.DeepLinkSchema)
		wv.Check(deepLinkSchema != "", "deep_link_schema", "deep_link_schema cannot be empty")

		deepLinkSchemaURL, err := url.ParseRequestURI(deepLinkSchema)
		if err != nil {
			log.Ctx(ctx).Errorf("parsing deep link schema: %v", err)
			wv.Check(false, "deep_link_schema", "invalid deep link schema provided")
		} else {
			validatedDeepLink := deepLinkSchemaURL.String()
			modifiedReq.DeepLinkSchema = &validatedDeepLink
		}
	}

	if reqBody.SEP10ClientDomain != nil {
		sep10ClientDomain := strings.TrimSpace(*reqBody.SEP10ClientDomain)
		wv.Check(sep10ClientDomain != "", "sep_10_client_domain", "sep_10_client_domain cannot be empty")

		sep10URL, err := url.Parse(sep10ClientDomain)
		if err != nil {
			log.Ctx(ctx).Errorf("parsing SEP-10 client domain URL: %v", err)
			wv.Check(false, "sep_10_client_domain", "invalid SEP-10 client domain URL provided")
		} else {
			sep10Host := sep10URL.Host
			if sep10Host == "" {
				sep10Host = sep10URL.String()
			}
			if err := utils.ValidateDNS(sep10Host); err != nil {
				log.Ctx(ctx).Errorf("validating SEP-10 client domain: %v", err)
				wv.Check(false, "sep_10_client_domain", "invalid SEP-10 client domain provided")
			} else {
				modifiedReq.SEP10ClientDomain = &sep10Host
			}
		}
	}

	if reqBody.Assets != nil {
		var processedAssets []AssetReference
		for i, asset := range *reqBody.Assets {
			inferredAsset := wv.inferAssetType(asset)
			if err := inferredAsset.Validate(); err != nil {
				wv.Check(false, fmt.Sprintf("assets[%d]", i), err.Error())
				continue
			}
			processedAssets = append(processedAssets, inferredAsset)
		}
		modifiedReq.Assets = &processedAssets
	}

	if wv.HasErrors() {
		return nil
	}

	return modifiedReq
}

func (wv *WalletValidator) inferAssetType(asset AssetReference) AssetReference {
	// If ID is provided, no inference needed
	if asset.ID != "" {
		return asset
	}

	// If type is already specified, no inference needed
	if asset.Type != "" {
		return asset
	}

	// Inference logic for backward compatibility
	result := asset

	if strings.ToUpper(asset.Code) == assets.XLMAssetCode && asset.Issuer == "" {
		result.Type = string(AssetReferenceTypeNative)
		result.Code = ""
		return result
	}

	// Classic asset detection: has both code and issuer
	if asset.Code != "" && asset.Issuer != "" {
		result.Type = string(AssetReferenceTypeClassic)
		return result
	}

	return result
}

func (ar AssetReference) Validate() error {
	if ar.ID != "" {
		if ar.Type != "" || ar.Code != "" || ar.Issuer != "" || ar.ContractID != "" {
			return fmt.Errorf("when 'id' is provided, other fields should not be present")
		}
		return nil
	}

	if ar.Type == "" {
		return fmt.Errorf("either 'id' or 'type' must be provided")
	}

	switch AssetReferenceType(ar.Type) {
	case AssetReferenceTypeClassic:
		if ar.Code == "" {
			return fmt.Errorf("'code' is required for classic asset")
		}
		if ar.Issuer == "" {
			return fmt.Errorf("'issuer' is required for classic asset")
		}

		if !strkey.IsValidEd25519PublicKey(ar.Issuer) {
			return fmt.Errorf("invalid issuer address format")
		}
	case AssetReferenceTypeNative:
		if ar.Code != "" || ar.Issuer != "" || ar.ContractID != "" {
			return fmt.Errorf("native asset should not have code, issuer, or contract_id")
		}
	case AssetReferenceTypeContract, AssetReferenceTypeFiat:
		return fmt.Errorf("assets are not implemented yet")
	default:
		return fmt.Errorf("invalid asset type: %s", ar.Type)
	}

	return nil
}

func (ar AssetReference) GetReferenceType() AssetReferenceType {
	if ar.ID != "" {
		return AssetReferenceTypeID
	}
	return AssetReferenceType(ar.Type)
}
