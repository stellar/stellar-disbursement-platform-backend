package httphandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/amount"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/go/txnbuild"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine"
	tssUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/utils"
)

const (
	feeMultiplierInStroops = 10_000
	stellarNativeAssetCode = "XLM"
)

var errCouldNotRemoveTrustline = errors.New("could not remove trustline")

type AssetsHandler struct {
	Models             *data.Models
	HorizonClient      horizonclient.ClientInterface
	SignatureService   engine.SignatureService
	GetPreconditionsFn func() txnbuild.Preconditions
}

type AssetRequest struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
}

// GetAssets returns a list of assets.
func (c AssetsHandler) GetAssets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	walletID := strings.TrimSpace(r.URL.Query().Get("wallet"))

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
	httpjson.Render(w, assets, httpjson.JSON)
}

// CreateAsset adds a new asset.
func (c AssetsHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	var assetRequest AssetRequest
	err := json.NewDecoder(r.Body).Decode(&assetRequest)
	if err != nil {
		httperror.BadRequest("invalid request body", err, nil).Render(w)
		return
	}

	v := validators.NewValidator()
	v.Check(assetRequest.Code != "", "code", "code is required")
	if strings.ToUpper(assetRequest.Code) != stellarNativeAssetCode {
		v.Check(assetRequest.Issuer != "", "issuer", "issuer is required")
		v.Check(strkey.IsValidEd25519PublicKey(assetRequest.Issuer), "issuer", "issuer is invalid")
	}

	if v.HasErrors() {
		httperror.BadRequest("Request invalid", err, v.Errors).Render(w)
		return
	}

	ctx := r.Context()

	asset, err := db.RunInTransactionWithResult(ctx, c.Models.DBConnectionPool, nil, func(dbTx db.DBTransaction) (*data.Asset, error) {
		insertedAsset, insertErr := c.Models.Assets.Insert(
			ctx,
			dbTx,
			assetRequest.Code,
			assetRequest.Issuer,
		)
		if insertErr != nil {
			return nil, fmt.Errorf("error inserting new asset: %w", insertErr)
		}

		trustlineErr := c.handleUpdateAssetTrustlineForDistributionAccount(ctx, &txnbuild.CreditAsset{
			Code:   assetRequest.Code,
			Issuer: assetRequest.Issuer,
		}, nil)
		if trustlineErr != nil {
			return nil, fmt.Errorf("error adding trustline for the distribution account: %w", trustlineErr)
		}

		return insertedAsset, nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httperror.Conflict("asset already exists", err, nil).Render(w)
			return
		}

		httperror.InternalError(ctx, "Cannot create new asset", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusCreated, asset, httpjson.JSON)
}

// DeleteAsset marks an asset for soft delete.
func (c AssetsHandler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
			return nil, fmt.Errorf("error performing soft delete on asset id %s: %w", assetID, deleteErr)
		}

		trustlineErr := c.handleUpdateAssetTrustlineForDistributionAccount(ctx, nil, &txnbuild.CreditAsset{
			Code:   deletedAsset.Code,
			Issuer: deletedAsset.Issuer,
		})
		if trustlineErr != nil {
			return nil, fmt.Errorf("error removing trustline: %w", trustlineErr)
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

func (c AssetsHandler) handleUpdateAssetTrustlineForDistributionAccount(ctx context.Context, assetToAddTrustline *txnbuild.CreditAsset, assetToRemoveTrustline *txnbuild.CreditAsset) error {
	if assetToAddTrustline == nil && assetToRemoveTrustline == nil {
		return fmt.Errorf("should provide at least one asset")
	}

	if assetToAddTrustline != nil && assetToRemoveTrustline != nil &&
		*assetToAddTrustline == *assetToRemoveTrustline {
		return fmt.Errorf("should provide different assets")
	}

	acc, err := c.HorizonClient.AccountDetail(horizonclient.AccountRequest{
		AccountID: c.SignatureService.DistributionAccount(),
	})
	if err != nil {
		return fmt.Errorf("getting distribution account details: %w", err)
	}

	changeTrustOperations := make([]*txnbuild.ChangeTrust, 0)
	// remove asset
	if assetToRemoveTrustline != nil && strings.ToUpper(assetToRemoveTrustline.Code) != stellarNativeAssetCode {
		for _, balance := range acc.Balances {
			if balance.Asset.Code == assetToRemoveTrustline.Code && balance.Asset.Issuer == assetToRemoveTrustline.Issuer {
				assetToRemoveTrustlineBalance, err := amount.ParseInt64(balance.Balance)
				if err != nil {
					return fmt.Errorf("converting asset to remove trustline balance to int64: %w", err)
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
					SourceAccount: c.SignatureService.DistributionAccount(),
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
				SourceAccount: c.SignatureService.DistributionAccount(),
			})
		}
	}

	// No operations to perform
	if len(changeTrustOperations) == 0 {
		log.Ctx(ctx).Warn("not performing either add or remove trustline")
		return nil
	}

	if err := c.submitChangeTrustTransaction(ctx, &acc, changeTrustOperations); err != nil {
		return fmt.Errorf("submitting change trust transaction: %w", err)
	}

	return nil
}

func (c AssetsHandler) submitChangeTrustTransaction(ctx context.Context, acc *horizon.Account, changeTrustOperations []*txnbuild.ChangeTrust) error {
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
				AccountID: c.SignatureService.DistributionAccount(),
				Sequence:  acc.Sequence,
			},
			IncrementSequenceNum: true,
			Operations:           operations,
			BaseFee:              txnbuild.MinBaseFee * feeMultiplierInStroops,
			Preconditions:        preconditions,
		},
	)
	if err != nil {
		return fmt.Errorf("creating change trust transaction: %w", err)
	}

	tx, err = c.SignatureService.SignStellarTransaction(ctx, tx, c.SignatureService.DistributionAccount())
	if err != nil {
		return fmt.Errorf("signing change trust transaction: %w", err)
	}

	_, err = c.HorizonClient.SubmitTransactionWithOptions(tx, horizonclient.SubmitTxOpts{SkipMemoRequiredCheck: true})
	if err != nil {
		return fmt.Errorf("submitting change trust transaction to network: %w", tssUtils.NewHorizonErrorWrapper(err))
	}

	return nil
}
