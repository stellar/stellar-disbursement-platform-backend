package stellar

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/stellar/stellar-rpc/protocol"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// RPCOptions contains the configuration options for the Stellar RPC server.
type RPCOptions struct {
	// URL of the Stellar RPC server where this application will communicate with.
	RPCUrl string
	// The key of the request header to be used for authentication with the RPC server.
	RPCRequestAuthHeaderKey string
	// The value of the request header to be used for authentication with the RPC server.
	RPCRequestAuthHeaderValue string
}

// SimulationErrorType represents the type of simulation error that occurred
type SimulationErrorType string

const (
	// Network/Transport errors
	SimulationErrorTypeNetwork SimulationErrorType = "network"

	// Transaction parsing errors
	SimulationErrorTypeTransactionInvalid SimulationErrorType = "transaction_invalid"

	// Authorization errors
	SimulationErrorTypeAuth SimulationErrorType = "auth"

	// Contract execution errors
	SimulationErrorTypeContractExecution SimulationErrorType = "contract_execution"

	// Resource errors
	SimulationErrorTypeResource SimulationErrorType = "resource"

	// Unknown simulation errors
	SimulationErrorTypeUnknown SimulationErrorType = "unknown"
)

// SimulationError represents a structured error from RPC simulation
type SimulationError struct {
	Type     SimulationErrorType
	Err      error
	Response *protocol.SimulateTransactionResponse
}

func (e *SimulationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("simulation %s error: %v", e.Type, e.Err)
	}
	return fmt.Sprintf("simulation %s error", e.Type)
}

func (e *SimulationError) Unwrap() error {
	return e.Err
}

func (e *SimulationError) IsRetryable() bool {
	return e != nil && slices.Contains([]SimulationErrorType{SimulationErrorTypeNetwork, SimulationErrorTypeResource}, e.Type)
}

// SimulationResult wraps the successful simulation response
type SimulationResult struct {
	Response protocol.SimulateTransactionResponse
}

// NewSimulationError creates a new SimulationError with automatic error categorization
func NewSimulationError(err error, response *protocol.SimulateTransactionResponse) *SimulationError {
	var errorType SimulationErrorType
	if err != nil {
		if response != nil && response.Error != "" {
			errorType = categorizeSimulationError(response.Error)
		} else {
			// All other errors default to network error
			errorType = SimulationErrorTypeNetwork
		}
	} else {
		errorType = SimulationErrorTypeUnknown
	}

	return &SimulationError{
		Type:     errorType,
		Err:      err,
		Response: response,
	}
}

func categorizeSimulationError(msg string) SimulationErrorType {
	if msg == "" {
		return SimulationErrorTypeUnknown
	}

	msgLower := strings.ToLower(msg)

	if isContractExecutionError(msgLower) {
		return SimulationErrorTypeContractExecution
	}
	if isResourceError(msgLower) {
		return SimulationErrorTypeResource
	}
	if isTransactionInvalidError(msgLower) {
		return SimulationErrorTypeTransactionInvalid
	}
	if isAuthError(msgLower) {
		return SimulationErrorTypeAuth
	}

	return SimulationErrorTypeUnknown
}

func isTransactionInvalidError(err string) bool {
	return utils.ContainsAny(err, "unmarshal", "parse", "decode", "invalid transaction")
}

func isAuthError(err string) bool {
	return utils.ContainsAny(err, "authorization", "signature", "unauthorized")
}

func isContractExecutionError(err string) bool {
	return utils.ContainsAny(err,
		"contract execution failed",
		"contract error",
		"contract panic",
		"hosterror: error(storage,",
		"contract already exists",
		"wasm does not exist",
		"existingvalue)",
		"missingvalue)")
}

func isResourceError(err string) bool {
	return utils.ContainsAny(err, "resource", "cpu limit", "memory limit", "instructions limit", "limit exceeded")
}

// RPCClient is an interface that defines the methods for interacting with Stellar RPC.
//
//go:generate mockery --name=RPCClient --case=underscore --structname=MockRPCClient --filename=rpc_client.go
type RPCClient interface {
	SimulateTransaction(ctx context.Context, request protocol.SimulateTransactionRequest) (*SimulationResult, *SimulationError)
}
