package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/circle"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/transactionsubmission/engine/signing"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

type Balance struct {
	Amount      string `json:"amount"`
	AssetCode   string `json:"asset_code"`
	AssetIssuer string `json:"asset_issuer"`
}

type GetBalanceResponse struct {
	Account  schema.TransactionAccount `json:"account"`
	Balances []Balance                 `json:"balances"`
}

type BalancesHandler struct {
	DistributionAccountResolver signing.DistributionAccountResolver
	CircleService               circle.ServiceInterface
	NetworkType                 utils.NetworkType
}

// Get returns the balances of the distribution account.
func (h BalancesHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	distAccount, err := h.DistributionAccountResolver.DistributionAccountFromContext(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve distribution account", err, nil).Render(w)
		return
	}

	if !distAccount.IsCircle() {
		errResponseMsg := fmt.Sprintf("This endpoint is only available for tenants using %v", schema.CirclePlatform)
		httperror.BadRequest(errResponseMsg, nil, nil).Render(w)
		return
	}

	if distAccount.Status == schema.AccountStatusPendingUserActivation {
		errResponseMsg := fmt.Sprintf("This organization is not configured to use %v", schema.CirclePlatform)
		httperror.BadRequest(errResponseMsg, nil, nil).Render(w)
		return
	}

	circleWallet, err := h.CircleService.GetWalletByID(ctx, distAccount.CircleWalletID)
	if err != nil {
		wrapCircleError(ctx, err).Render(w)
		return
	}

	balances := h.filterBalances(ctx, circleWallet)

	response := GetBalanceResponse{
		Account:  distAccount,
		Balances: balances,
	}
	httpjson.Render(w, response, httpjson.JSON)
}

func (h BalancesHandler) filterBalances(ctx context.Context, circleWallet *circle.Wallet) []Balance {
	balances := []Balance{}
	for _, balance := range circleWallet.Balances {
		networkAssetMap, ok := circle.AllowedAssetsMap[balance.Currency]
		if !ok {
			log.Ctx(ctx).Debugf("Ignoring balance for asset %s, as it's not supported by the SDP", balance.Currency)
			continue
		}

		asset, ok := networkAssetMap[h.NetworkType]
		if !ok {
			log.Ctx(ctx).Debugf("Ignoring balance for asset %s, as it's not supported by the SDP in the %v network", balance.Currency, h.NetworkType)
			continue
		}

		balances = append(balances, Balance{
			Amount:      balance.Amount,
			AssetCode:   asset.Code,
			AssetIssuer: asset.Issuer,
		})
	}
	return balances
}

func wrapCircleError(ctx context.Context, err error) *httperror.HTTPError {
	if err == nil {
		return nil
	}

	var circleAPIErr *circle.APIError
	if errors.As(err, &circleAPIErr) {
		extras := map[string]interface{}{"circle_errors": circleAPIErr.Errors}
		msg := fmt.Sprintf("Cannot complete Circle request: %s", circleAPIErr.Message)
		return httperror.BadRequest(msg, circleAPIErr, extras)
	}
	return httperror.InternalError(ctx, "Cannot complete Circle request", err, nil)
}
