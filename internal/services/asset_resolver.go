package services

import (
	"context"
	"fmt"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type WalletAssetResolver struct {
	assetModel *data.AssetModel
}

func NewWalletAssetResolver(assetModel *data.AssetModel) *WalletAssetResolver {
	return &WalletAssetResolver{
		assetModel: assetModel,
	}
}

func (ar *WalletAssetResolver) ResolveAssetReferences(ctx context.Context, references []validators.AssetReference) ([]string, error) {
	assetIDs := make([]string, 0, len(references))

	for i, ref := range references {
		assetID, err := ar.resolveAssetReference(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve asset reference at index %d: %w", i, err)
		}
		assetIDs = append(assetIDs, assetID)
	}

	return assetIDs, nil
}

func (ar *WalletAssetResolver) resolveAssetReference(ctx context.Context, ref validators.AssetReference) (string, error) {
	switch ref.GetReferenceType() {
	case validators.AssetReferenceTypeID:
		asset, err := ar.assetModel.Get(ctx, ref.ID)
		if err != nil {
			if err == data.ErrRecordNotFound {
				return "", fmt.Errorf("asset with ID %s not found", ref.ID)
			}
			return "", fmt.Errorf("failed to get asset by ID: %w", err)
		}
		return asset.ID, nil
	case validators.AssetReferenceTypeClassic:
		asset, err := ar.assetModel.GetOrCreate(ctx, ref.Code, ref.Issuer)
		if err != nil {
			return "", fmt.Errorf("failed to get or create classic asset: %w", err)
		}
		return asset.ID, nil
	case validators.AssetReferenceTypeNative:
		asset, err := ar.assetModel.GetOrCreate(ctx, "XLM", "")
		if err != nil {
			return "", fmt.Errorf("failed to get or create native asset: %w", err)
		}
		return asset.ID, nil
	case validators.AssetReferenceTypeContract, validators.AssetReferenceTypeFiat:
		return "", fmt.Errorf("assets are not implemented yet")
	default:
		return "", fmt.Errorf("unknown asset reference type")
	}
}

func (ar *WalletAssetResolver) ValidateAssetIDs(ctx context.Context, assetIDs []string) error {
	for _, assetID := range assetIDs {
		_, err := ar.assetModel.Get(ctx, assetID)
		if err != nil {
			if err == data.ErrRecordNotFound {
				return fmt.Errorf("asset with ID %s not found", assetID)
			}
			return fmt.Errorf("failed to validate asset ID %s: %w", assetID, err)
		}
	}
	return nil
}
