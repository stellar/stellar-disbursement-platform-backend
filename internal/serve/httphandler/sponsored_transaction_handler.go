package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/support/http/httpdecode"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

type SponsoredTransactionHandler struct {
	EmbeddedWalletService services.EmbeddedWalletServiceInterface
	Models                *data.Models
	NetworkPassphrase     string
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
	account = strings.TrimSpace(account)
	if account == "" {
		httperror.Unauthorized("", services.ErrMissingAccount, nil).Render(w)
		return
	}

	contractAddress, err := getInvokeContractAddress(reqBody.OperationXDR)
	if err != nil {
		httperror.BadRequest("operation_xdr must target a valid contract address", err, nil).Render(w)
		return
	}

	allowed, err := h.isContractAllowed(ctx, contractAddress)
	if err != nil {
		httperror.InternalError(ctx, "Failed to validate contract address", err, nil).Render(w)
		return
	}
	if !allowed {
		httperror.BadRequest("operation_xdr contract address is not supported for this tenant", nil, nil).Render(w)
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
	account, err := sdpcontext.GetWalletContractAddressFromContext(ctx)
	if err != nil {
		httperror.Unauthorized("Wallet contract address not found in context", err, nil).Render(w)
		return
	}
	account = strings.TrimSpace(account)
	if account == "" {
		httperror.Unauthorized("", services.ErrMissingAccount, nil).Render(w)
		return
	}
	transactionID := strings.TrimSpace(chi.URLParam(r, "id"))

	if transactionID == "" {
		httperror.BadRequest("Transaction ID is required", nil, nil).Render(w)
		return
	}

	transaction, err := h.EmbeddedWalletService.GetTransactionStatus(ctx, account, transactionID)
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

// getInvokeContractAddress extracts the contract address from an InvokeHostFunctionOp XDR payload.
func getInvokeContractAddress(operationXDR string) (string, error) {
	var invoke xdr.InvokeHostFunctionOp
	if err := xdr.SafeUnmarshalBase64(operationXDR, &invoke); err != nil {
		return "", fmt.Errorf("decoding operation XDR: %w", err)
	}

	if invoke.HostFunction.Type != xdr.HostFunctionTypeHostFunctionTypeInvokeContract {
		return "", fmt.Errorf("operation is not an invoke contract host function")
	}
	if invoke.HostFunction.InvokeContract == nil {
		return "", fmt.Errorf("invoke contract details are missing")
	}

	contractAddress := invoke.HostFunction.InvokeContract.ContractAddress
	if contractAddress.Type != xdr.ScAddressTypeScAddressTypeContract || contractAddress.ContractId == nil {
		return "", fmt.Errorf("invoke contract address is not a contract address")
	}

	contractID := *contractAddress.ContractId
	encoded, err := strkey.Encode(strkey.VersionByteContract, contractID[:])
	if err != nil {
		return "", fmt.Errorf("encoding contract address: %w", err)
	}

	return encoded, nil
}

// isContractAllowed checks if the contract address is allowed. Only SACs enabled for the wallet are allowed.
func (h SponsoredTransactionHandler) isContractAllowed(ctx context.Context, contractAddress string) (bool, error) {
	if strings.TrimSpace(contractAddress) == "" {
		return false, fmt.Errorf("contract address is required")
	}
	if h.Models == nil {
		return false, fmt.Errorf("models are required")
	}
	networkPassphrase := strings.TrimSpace(h.NetworkPassphrase)
	if networkPassphrase == "" {
		return false, fmt.Errorf("network passphrase is required")
	}

	assets, err := h.getEmbeddedWalletAssets(ctx)
	if err != nil {
		return false, fmt.Errorf("getting embedded wallet assets: %w", err)
	}

	for _, asset := range assets {
		var xdrAsset xdr.Asset
		if asset.IsNative() {
			xdrAsset = xdr.MustNewNativeAsset()
		} else {
			var buildErr error
			xdrAsset, buildErr = xdr.NewCreditAsset(asset.Code, asset.Issuer)
			if buildErr != nil {
				return false, fmt.Errorf("building asset %s:%s: %w", asset.Code, asset.Issuer, buildErr)
			}
		}

		contractID, err := xdrAsset.ContractID(networkPassphrase)
		if err != nil {
			return false, fmt.Errorf("calculating contract ID for asset %s:%s: %w", asset.Code, asset.Issuer, err)
		}
		encoded, err := strkey.Encode(strkey.VersionByteContract, contractID[:])
		if err != nil {
			return false, fmt.Errorf("encoding contract ID for asset %s:%s: %w", asset.Code, asset.Issuer, err)
		}

		if encoded == contractAddress {
			return true, nil
		}
	}

	return false, nil
}

// getEmbeddedWalletAssets retrieves the assets associated with the embedded wallet.
func (h SponsoredTransactionHandler) getEmbeddedWalletAssets(ctx context.Context) ([]data.Asset, error) {
	wallets, err := h.Models.Wallets.FindWallets(ctx, data.Filter{Key: data.FilterEmbedded, Value: true})
	if err != nil {
		return nil, fmt.Errorf("finding embedded wallets: %w", err)
	}

	if len(wallets) != 1 {
		return nil, fmt.Errorf("expected exactly one embedded wallet, found %d", len(wallets))
	}

	assets, err := h.Models.Wallets.GetAssets(ctx, wallets[0].ID)
	if err != nil {
		return nil, fmt.Errorf("getting embedded wallet assets: %w", err)
	}

	return assets, nil
}
