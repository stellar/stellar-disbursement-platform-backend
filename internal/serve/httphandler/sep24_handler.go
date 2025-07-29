package httphandler

import (
	"errors"
	"net/http"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type SEP24Handler struct {
	Models *data.Models
}

type SEP24TransactionResponse struct {
	Transaction SEP24Transaction `json:"transaction"`
}

type SEP24Transaction struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

func (h SEP24Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	transactionID := r.URL.Query().Get("id")
	if transactionID == "" {
		httperror.BadRequest("id parameter is required", nil, nil).Render(w)
		return
	}

	receiverWallet, err := h.Models.ReceiverWallet.GetByAnchorPlatformTransactionID(ctx, transactionID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("transaction not found", err, nil).Render(w)
			return
		}
		log.Ctx(ctx).Errorf("error getting receiver wallet by transaction ID: %v", err)
		httperror.InternalError(ctx, "Failed to get transaction status", err, nil).Render(w)
		return
	}

	var status string
	switch receiverWallet.Status {
	case data.ReadyReceiversWalletStatus:
		status = "pending_external"
	case data.RegisteredReceiversWalletStatus:
		status = "completed"
	default:
		status = "error"
	}

	response := SEP24TransactionResponse{
		Transaction: SEP24Transaction{
			ID:     transactionID,
			Kind:   "deposit",
			Status: status,
		},
	}

	httpjson.Render(w, response, httpjson.JSON)
}

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
	Enabled   bool `json:"enabled"`
	MinAmount int  `json:"min_amount"`
	MaxAmount int  `json:"max_amount"`
}

type SEP24FeeResponse struct {
	Enabled bool `json:"enabled"`
}

type SEP24FeatureFlagResponse struct {
	AccountCreation   bool `json:"account_creation"`
	ClaimableBalances bool `json:"claimable_balances"`
}

func (h SEP24Handler) GetInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	assets, err := h.Models.Assets.GetAll(ctx)
	if err != nil {
		log.Ctx(ctx).Errorf("Error fetching assets for SEP-24 info: %v", err)
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
			MinAmount: sep24MinAmount,
			MaxAmount: sep24MaxAmount,
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
