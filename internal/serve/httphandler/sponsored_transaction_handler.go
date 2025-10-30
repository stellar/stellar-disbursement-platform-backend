package httphandler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/go/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

type SponsoredTransactionHandler struct {
	EmbeddedWalletService services.EmbeddedWalletServiceInterface
}

type CreateSponsoredTransactionRequest struct {
	OperationXDR string `json:"operation_xdr"`
}

func (r CreateSponsoredTransactionRequest) Validate() *httperror.HTTPError {
	validator := validators.NewValidator()

	validator.Check(len(strings.TrimSpace(r.OperationXDR)) > 0, "operation_xdr", "operation_xdr should not be empty")
	if r.OperationXDR != "" {
		var invoke xdr.InvokeHostFunctionOp
		if err := xdr.SafeUnmarshalBase64(r.OperationXDR, &invoke); err != nil {
			validator.AddError("operation_xdr", "operation_xdr is not valid base64-encoded InvokeHostFunctionOp XDR")
		}
	}

	if validator.HasErrors() {
		return httperror.BadRequest("", nil, validator.Errors)
	}

	return nil
}

type CreateSponsoredTransactionResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type GetSponsoredTransactionResponse struct {
	Status          string `json:"status"`
	TransactionHash string `json:"transaction_hash,omitempty"`
}

func (h SponsoredTransactionHandler) CreateSponsoredTransaction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var reqBody CreateSponsoredTransactionRequest
	if err := httpdecode.DecodeJSON(r, &reqBody); err != nil {
		httperror.BadRequest("", err, nil).Render(w)
		return
	}

	if err := reqBody.Validate(); err != nil {
		err.Render(w)
		return
	}

	account, err := sdpcontext.GetWalletContractAddressFromContext(ctx)
	if err != nil {
		httperror.Unauthorized("Wallet contract address not found in context", err, nil).Render(w)
		return
	}

	transactionID, err := h.EmbeddedWalletService.SponsorTransaction(ctx, account, reqBody.OperationXDR)
	if err != nil {
		httperror.InternalError(ctx, "Failed to create sponsored transaction", err, nil).Render(w)
		return
	}

	resp := CreateSponsoredTransactionResponse{
		ID:     transactionID,
		Status: string(data.PendingSponsoredTransactionStatus),
	}

	httpjson.RenderStatus(w, http.StatusAccepted, resp, httpjson.JSON)
}

func (h SponsoredTransactionHandler) GetSponsoredTransaction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	transactionID := strings.TrimSpace(chi.URLParam(r, "id"))

	if transactionID == "" {
		httperror.BadRequest("Transaction ID is required", nil, nil).Render(w)
		return
	}

	transaction, err := h.EmbeddedWalletService.GetTransactionStatus(ctx, transactionID)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("Transaction not found", err, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "Failed to get transaction", err, nil).Render(w)
		}
		return
	}

	resp := GetSponsoredTransactionResponse{
		Status:          transaction.Status,
		TransactionHash: transaction.TransactionHash,
	}

	httpjson.Render(w, resp, httpjson.JSON)
}
