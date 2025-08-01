package httphandler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

const RotateSignerFnName = "rotate_signer"

type CosignRecoveryHandler struct {
	Models            *data.Models
	RotationSecretKey string
	NetworkPassphrase string
}

type CosignRecoveryRequest struct {
	TransactionXDR string `json:"transaction_xdr"`
}

type CosignRecoveryResponse struct {
	SignedTransactionXDR string `json:"signed_transaction_xdr"`
}

func (h CosignRecoveryHandler) CosignRecovery(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Get contract address from URL parameter
	contractAddress := strings.TrimSpace(chi.URLParam(req, "contractAddress"))
	if len(contractAddress) == 0 {
		httperror.BadRequest("Contract address is required", nil, nil).Render(rw)
		return
	}

	// Check if contract exists in database
	_, err := h.Models.EmbeddedWallets.GetByContractAddress(ctx, nil, contractAddress)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("Contract not found", err, nil).Render(rw)
		} else {
			httperror.InternalError(ctx, "Failed to get wallet", err, nil).Render(rw)
		}
		return
	}

	var reqBody CosignRecoveryRequest
	if err = httpdecode.DecodeJSON(req, &reqBody); err != nil {
		httperror.BadRequest("Decoding request body", err, nil).Render(rw)
		return
	}

	// Parse and validate the transaction:
	tx, err := parseAndValidateTransaction(reqBody.TransactionXDR, contractAddress)
	if err != nil {
		httperror.BadRequest(err.Error(), err, nil).Render(rw)
		return
	}

	txXDR, err := h.signTransaction(ctx, tx)
	if err != nil {
		httperror.InternalError(ctx, err.Error(), err, nil).Render(rw)
		return
	}

	resp := CosignRecoveryResponse{SignedTransactionXDR: txXDR}
	httpjson.Render(rw, resp, httpjson.JSON)
}

// parseAndValidateTransaction parses the transaction XDR and performs a series of security validations:
// - The transaction must have exactly one operation
// - The operation must be an invoke host function
// - The invoke host function must be a contract invocation
// - The contract invocation must be to the intended (url param) contract
// - The contract invocation must be to the `rotate_signer` function
// - The contract invocation must not have subinvocations
func parseAndValidateTransaction(txXDR, intendedContractID string) (*txnbuild.Transaction, error) {
	if len(txXDR) == 0 {
		return nil, errors.New("transaction_xdr cannot be empty")
	}

	// Parse and validate the transaction:
	genericTx, err := txnbuild.TransactionFromXDR(txXDR)
	if err != nil {
		return nil, fmt.Errorf("invalid transaction XDR: %w", err)
	}

	tx, ok := genericTx.Transaction()
	if !ok {
		return nil, errors.New("generic transaction could not be converted into transaction, ensure you're not sending a fee bump transaction")
	}

	ops := tx.Operations()
	if len(ops) != 1 {
		return nil, errors.New("transaction must have exactly one operation")
	}

	invokeHostOp, ok := ops[0].(*txnbuild.InvokeHostFunction)
	if !ok {
		return nil, errors.New("transaction operation is not an invoke host function")
	}

	if invokeHostOp.HostFunction.Type != xdr.HostFunctionTypeHostFunctionTypeInvokeContract {
		return nil, errors.New("invoke host function is not a contract invocation")
	}

	invokeContractOp := invokeHostOp.HostFunction.MustInvokeContract()
	invokedContractID, err := invokeContractOp.ContractAddress.String()
	if err != nil {
		return nil, fmt.Errorf("invalid invoked contract address: %w", err)
	}

	if invokedContractID != intendedContractID {
		return nil, fmt.Errorf("wrong contract being invoked: %s != %s", invokedContractID, intendedContractID)
	}

	if invokeContractOp.FunctionName != RotateSignerFnName {
		return nil, fmt.Errorf("wrong function being called: %s != %s", invokeContractOp.FunctionName, RotateSignerFnName)
	}

	for _, sorobanAuth := range invokeHostOp.Auth {
		if len(sorobanAuth.RootInvocation.SubInvocations) > 0 {
			return nil, errors.New("contract operation has subinvocations which are not allowed")
		}
	}

	return tx, nil
}

func (h CosignRecoveryHandler) signTransaction(_ context.Context, tx *txnbuild.Transaction) (string, error) {
	recoveryKP, err := keypair.ParseFull(h.RotationSecretKey)
	if err != nil {
		return "", fmt.Errorf("invalid admin secret key configuration: %w", err)
	}

	tx, err = tx.Sign(h.NetworkPassphrase, recoveryKP)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	txXDR, err := tx.Base64()
	if err != nil {
		return "", fmt.Errorf("failed to encode signed transaction: %w", err)
	}

	return txXDR, nil
}
