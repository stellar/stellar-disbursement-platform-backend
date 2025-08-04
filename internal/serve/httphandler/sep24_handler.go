package httphandler

import (
	"net/http"
	"strconv"

	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

const (
	sep24MinAmount = 1
	sep24MaxAmount = 10000
)

type SEP24InfoResponse struct {
	Deposit  map[string]SEP24OperationResponse `json:"deposit"`
	Withdraw map[string]SEP24OperationResponse `json:"withdraw"`
	Fee      SEP24FeeResponse                  `json:"fee"`
	Features SEP24FeatureFlagResponse          `json:"features"`
}

type SEP24OperationResponse struct {
	Enabled   bool   `json:"enabled"`
	MinAmount string `json:"min_amount"`
	MaxAmount string `json:"max_amount"`
}

type SEP24FeeResponse struct {
	Enabled bool `json:"enabled"`
}

type SEP24FeatureFlagResponse struct {
	AccountCreation   bool `json:"account_creation"`
	ClaimableBalances bool `json:"claimable_balances"`
}

type SEP24InfoHandler struct {
	Models *data.Models
}

func (h SEP24InfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	assets, err := h.Models.Assets.GetAll(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot retrieve assets", err, nil).Render(w)
		return
	}

	deposit := make(map[string]SEP24OperationResponse)

	for _, asset := range assets {
		assetCode := asset.Code
		if asset.IsNative() {
			assetCode = "native"
		}

		deposit[assetCode] = SEP24OperationResponse{
			Enabled:   true,
			MinAmount: strconv.Itoa(sep24MinAmount),
			MaxAmount: strconv.Itoa(sep24MaxAmount),
		}
	}

	response := SEP24InfoResponse{
		Deposit:  deposit,
		Withdraw: make(map[string]SEP24OperationResponse),
		Fee: SEP24FeeResponse{
			Enabled: false,
		},
		Features: SEP24FeatureFlagResponse{
			AccountCreation:   false,
			ClaimableBalances: false,
		},
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}
