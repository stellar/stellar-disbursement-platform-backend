package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/amount"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

const stellarNativeAssetCode = "XLM"

var errCouldNotRemoveTrustline = errors.New("could not remove trustline")

type AssetsHandler struct {
	Models *data.Models
	engine.SubmitterEngine
	GetPreconditionsFn func() txnbuild.Preconditions
}

type AssetRequest struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
}

// AssetWithTrustlineInfo represents an asset with trustline information.
type AssetWithTrustlineInfo struct {
	data.Asset
	HasTrustline bool `json:"has_trustline"`
}

// GetAssets returns a list of assets.
func (c AssetsHandler) GetAssets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	walletID := strings.TrimSpace(r.URL.Query().Get("wallet"))
	hasTrustlineParam := strings.TrimSpace(r.URL.Query().Get("hasTrustline"))

	var assets []data.Asset
	var err error
	if walletID != "" {
		assets, err = c.Models.Assets.GetByWalletID(ctx, walletID)
	} else {
		assets, err = c.Models.Assets.GetAll(ctx)
	}
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve assets", err, nil).Render(w)
		return
	}

	// If hasTrustline parameter is provided, filter assets by trustline availability.
	if hasTrustlineParam != "" {
		hasTrustline, err := strconv.ParseBool(hasTrustlineParam)
		if err != nil {
			httperror.BadRequest("Invalid hasTrustline parameter. Must be 'true' or 'false'", err, nil).Render(w)
			return
		}

		distributionAccount, err := c.DistributionAccountFromContext(ctx)
		if err != nil {
			httperror.InternalError(ctx, "Cannot resolve distribution account from context", err, nil).Render(w)
			return
		}

		responseAssets := make([]AssetWithTrustlineInfo, 0)
		for _, asset := range assets {
			hasAssetTrustline, err := c.checkTrustlineExists(&distributionAccount, asset)
			if err != nil {
				log.Ctx(ctx).Warnf("Error checking trustline for asset %s:%s: %v", asset.Code, asset.Issuer, err)
				continue
			}

			if hasAssetTrustline == hasTrustline {
				responseAssets = append(responseAssets, AssetWithTrustlineInfo{
					Asset:        asset,
					HasTrustline: hasAssetTrustline,
				})
			}
		}

		httpjson.Render(w, responseAssets, httpjson.JSON)
		return
	}

	httpjson.Render(w, assets, httpjson.JSON)
}

// checkTrustlineExists checks if the distribution account has a trustline for the given asset.
func (c AssetsHandler) checkTrustlineExists(
	account *schema.TransactionAccount,
	asset data.Asset,
) (bool, error) {
	if asset.IsNative() {
		return true, nil
	}

	if !account.IsStellar() {
		// For Circle accounts, check if the asset is supported by Circle
		for _, networkAssets := range circle.AllowedAssetsMap {
			for _, circleAsset := range networkAssets {
				if circleAsset.Code == asset.Code && circleAsset.Issuer == asset.Issuer {
					return true, nil
				}
			}
		}
		return false, nil
	}

	acc, err := c.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: account.Address,
	})
	if err != nil {
		if horizonErr, ok := err.(*horizonclient.Error); ok {
			if horizonErr.Response.StatusCode == 404 {
				return false, fmt.Errorf("account %s not found on the Stellar network", account.Address)
			}
		}
		return false, fmt.Errorf("getting account details from Horizon: %w", err)
	}

	for _, balance := range acc.Balances {
		if balance.Asset.Type == validators.AssetTypeNative {
			continue
		}

		if balance.Asset.Code == asset.Code && balance.Asset.Issuer == asset.Issuer {
			return true, nil
		}
	}
	return false, nil
}

// CreateAsset adds a new asset.
func (c AssetsHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	distributionAccount, err := c.DistributionAccountFromContext(ctx)
	if err != nil {
		err = fmt.Errorf("resolving distribution account from context: %w", err)
		httperror.InternalError(ctx, "Cannot resolve distribution account from context", err, nil).Render(w)
		return
	} else if !distributionAccount.IsStellar() {
		httperror.BadRequest("Distribution account affiliated with tenant is not a Stellar account", nil, nil).Render(w)
		return
	}

	var assetRequest AssetRequest
	err = json.NewDecoder(r.Body).Decode(&assetRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	assetCode := strings.TrimSpace(strings.ToUpper(assetRequest.Code))
	assetIssuer := strings.TrimSpace(assetRequest.Issuer)

	v := validators.NewValidator()
	v.Check(assetCode != "", "code", "code is required")
	if assetCode != stellarNativeAssetCode {
		v.Check(strkey.IsValidEd25519PublicKey(assetIssuer), "issuer", "issuer is invalid")
	}

	if v.HasErrors() {
		httperror.BadRequest("Request invalid", err, v.Errors).Render(w)
		return
	}

	asset, err := db.RunInTransactionWithResult(ctx, c.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.Asset, error) {
		insertedAsset, insertErr := c.Models.Assets.Insert(ctx, dbTx, assetCode, assetIssuer)
		if insertErr != nil {
			return nil, fmt.Errorf("inserting new asset: %w", insertErr)
		}

		assetToAdd := &txnbuild.CreditAsset{Code: assetCode, Issuer: assetIssuer}
		trustlineErr := c.handleUpdateAssetTrustlineForDistributionAccount(ctx, assetToAdd, nil, distributionAccount)
		if trustlineErr != nil {
			return nil, fmt.Errorf("adding trustline for the distribution account: %w", trustlineErr)
		}

		return insertedAsset, nil
	})
	if err != nil {
		err = fmt.Errorf("creating asset in AssetHandler: %w", err)
		httperror.InternalError(ctx, "Cannot create new asset", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusCreated, asset, httpjson.JSON)
}

// DeleteAsset marks an asset for soft delete.
func (c AssetsHandler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	distributionAccount, err := c.DistributionAccountFromContext(ctx)
	if err != nil {
		err = fmt.Errorf("resolving distribution account from context: %w", err)
		httperror.InternalError(ctx, "Cannot resolve distribution account from context", err, nil).Render(w)
		return
	} else if !distributionAccount.IsStellar() {
		httperror.BadRequest("Distribution account affiliated with tenant is not a Stellar account", nil, nil).Render(w)
		return
	}

	assetID := chi.URLParam(r, "id")

	asset, err := c.Models.Assets.Get(ctx, assetID)
	if err != nil {
		log.Ctx(ctx).Errorf("Error performing soft delete on asset id %s: %s", assetID, err.Error())
		httperror.NotFound("could not find asset for deletion", err, nil).Render(w)
		return
	}

	if asset.DeletedAt != nil {
		log.Ctx(ctx).Errorf("Error performing soft delete on asset id %s: %s", assetID, "asset already deleted")
		httpjson.RenderStatus(w, http.StatusNoContent, "asset already deleted", httpjson.JSON)
		return
	}

	asset, err = db.RunInTransactionWithResult(ctx, c.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.Asset, error) {
		deletedAsset, deleteErr := c.Models.Assets.SoftDelete(ctx, dbTx, assetID)
		if deleteErr != nil {
			return nil, fmt.Errorf("performing soft delete on asset id %s: %w", assetID, deleteErr)
		}

		assetToRemove := &txnbuild.CreditAsset{Code: deletedAsset.Code, Issuer: deletedAsset.Issuer}
		trustlineErr := c.handleUpdateAssetTrustlineForDistributionAccount(ctx, nil, assetToRemove, distributionAccount)
		if trustlineErr != nil {
			return nil, fmt.Errorf("removing trustline: %w", trustlineErr)
		}

		return asset, nil
	})
	if err != nil {
		if errors.Is(err, errCouldNotRemoveTrustline) {
			httperror.UnprocessableEntity("Could not remove trustline because distribution account still has balance", err, nil).Render(w)
			return
		}

		httperror.InternalError(ctx, "Cannot delete asset", err, nil).Render(w)
		return
	}

	httpjson.Render(w, asset, httpjson.JSON)
}

func (c AssetsHandler) handleUpdateAssetTrustlineForDistributionAccount(
	ctx context.Context,
	assetToAddTrustline *txnbuild.CreditAsset,
	assetToRemoveTrustline *txnbuild.CreditAsset,
	distributionAccount schema.TransactionAccount,
) error {
	// Non-native Stellar distribution accounts will not require asset trustlines to be managed on our end. This is
	// technically unreachable from the endpoint entry points, but we will still check for this case here.
	if !distributionAccount.IsStellar() {
		return fmt.Errorf("distribution account is not a native Stellar account")
	}

	if assetToAddTrustline == nil && assetToRemoveTrustline == nil {
		return fmt.Errorf("should provide at least one asset")
	}

	if assetToAddTrustline != nil && assetToRemoveTrustline != nil &&
		*assetToAddTrustline == *assetToRemoveTrustline {
		return fmt.Errorf("should provide different assets")
	}

	acc, err := c.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: distributionAccount.Address,
	})
	if err != nil {
		return fmt.Errorf("getting distribution account details: %w", err)
	}

	changeTrustOperations := make([]*txnbuild.ChangeTrust, 0)
	// remove asset
	if assetToRemoveTrustline != nil && strings.ToUpper(assetToRemoveTrustline.Code) != stellarNativeAssetCode {
		for _, balance := range acc.Balances {
			if balance.Asset.Code == assetToRemoveTrustline.Code && balance.Asset.Issuer == assetToRemoveTrustline.Issuer {
				assetToRemoveTrustlineBalance, parseBalErr := amount.ParseInt64(balance.Balance)
				if parseBalErr != nil {
					return fmt.Errorf("converting asset to remove trustline balance to int64: %w", parseBalErr)
				}
				if assetToRemoveTrustlineBalance > 0 {
					log.Ctx(ctx).Warnf(
						"not removing trustline for the asset %s:%s because the distribution account still has balance: %s %s",
						assetToRemoveTrustline.Code, assetToRemoveTrustline.Issuer,
						amount.StringFromInt64(assetToRemoveTrustlineBalance), assetToRemoveTrustline.Code,
					)
					return errCouldNotRemoveTrustline
				}

				log.Ctx(ctx).Infof("removing trustline for asset %s:%s", assetToRemoveTrustline.Code, assetToRemoveTrustline.Issuer)
				changeTrustOperations = append(changeTrustOperations, &txnbuild.ChangeTrust{
					Line: txnbuild.ChangeTrustAssetWrapper{
						Asset: *assetToRemoveTrustline,
					},
					Limit:         "0", // 0 means remove trustline
					SourceAccount: distributionAccount.Address,
				})

				break
			}
		}

		if len(changeTrustOperations) == 0 {
			log.Ctx(ctx).Warnf(
				"not removing trustline for the asset %s:%s because it could not be found on the blockchain",
				assetToRemoveTrustline.Code, assetToRemoveTrustline.Issuer,
			)
		}
	}

	// add asset
	if assetToAddTrustline != nil && strings.ToUpper(assetToAddTrustline.Code) != stellarNativeAssetCode {
		var assetToAddTrustlineFound bool
		for _, balance := range acc.Balances {
			if balance.Asset.Code == assetToAddTrustline.Code && balance.Asset.Issuer == assetToAddTrustline.Issuer {
				assetToAddTrustlineFound = true
				log.Ctx(ctx).Warnf("not adding trustline for the asset %s:%s because it already exists", assetToAddTrustline.Code, assetToAddTrustline.Issuer)
				break
			}
		}

		if !assetToAddTrustlineFound {
			log.Ctx(ctx).Infof("adding trustline for asset %s:%s", assetToAddTrustline.Code, assetToAddTrustline.Issuer)
			changeTrustOperations = append(changeTrustOperations, &txnbuild.ChangeTrust{
				Line: txnbuild.ChangeTrustAssetWrapper{
					Asset: *assetToAddTrustline,
				},
				Limit:         "", // empty means no limit
				SourceAccount: distributionAccount.Address,
			})
		}
	}

	// No operations to perform
	if len(changeTrustOperations) == 0 {
		log.Ctx(ctx).Warn("not performing either add or remove trustline")
		return nil
	}

	if err = c.submitChangeTrustTransaction(ctx, &acc, changeTrustOperations, distributionAccount); err != nil {
		return fmt.Errorf("submitting change trust transaction: %w", err)
	}

	return nil
}

func (c AssetsHandler) submitChangeTrustTransaction(
	ctx context.Context, acc *horizon.Account, changeTrustOperations []*txnbuild.ChangeTrust, distributionAccount schema.TransactionAccount,
) error {
	if len(changeTrustOperations) < 1 {
		return fmt.Errorf("should have at least one change trust operation")
	}

	operations := make([]txnbuild.Operation, 0, len(changeTrustOperations))
	for _, ctOp := range changeTrustOperations {
		operations = append(operations, ctOp)
	}

	preconditions := txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(20)}
	if c.GetPreconditionsFn != nil {
		preconditions = c.GetPreconditionsFn()
	}
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount: &txnbuild.SimpleAccount{
				AccountID: distributionAccount.Address,
				Sequence:  acc.Sequence,
			},
			IncrementSequenceNum: true,
			Operations:           operations,
			BaseFee:              int64(c.MaxBaseFee),
			Preconditions:        preconditions,
		},
	)
	if err != nil {
		return fmt.Errorf("creating change trust transaction: %w", err)
	}

	tx, err = c.SignerRouter.SignStellarTransaction(ctx, tx, distributionAccount)
	if err != nil {
		return fmt.Errorf("signing change trust transaction: %w", err)
	}

	_, err = c.HorizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		return fmt.Errorf("submitting change trust transaction to network: %w", tssUtils.NewHorizonErrorWrapper(err))
	}

	return nil
}
