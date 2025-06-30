package stellar

import (
	"context"
	"fmt"
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
	Type        SimulationErrorType
	Message     string
	OriginalErr error
	IsRetryable bool
	Response    *protocol.SimulateTransactionResponse
}

func (e *SimulationError) Error() string {
	if e.OriginalErr != nil {
		return fmt.Sprintf("simulation %s error, %s (original: %v)", e.Type, e.Message, e.OriginalErr)
	}
	return fmt.Sprintf("simulation %s error, %s", e.Type, e.Message)
}

func (e *SimulationError) Unwrap() error {
	return e.OriginalErr
}

// SimulationResult wraps the successful simulation response
type SimulationResult struct {
	Response protocol.SimulateTransactionResponse
}

// NewSimulationError creates a new SimulationError with the given type and message
func NewSimulationError(errorType SimulationErrorType, message string, originalErr error, response *protocol.SimulateTransactionResponse) *SimulationError {
	isRetryable := determineRetryability(errorType)

	return &SimulationError{
		Type:        errorType,
		Message:     message,
		OriginalErr: originalErr,
		IsRetryable: isRetryable,
		Response:    response,
	}
}

func determineRetryability(errorType SimulationErrorType) bool {
	switch errorType {
	case SimulationErrorTypeNetwork, SimulationErrorTypeResource:
		return true
	case SimulationErrorTypeTransactionInvalid, SimulationErrorTypeAuth, SimulationErrorTypeContractExecution, SimulationErrorTypeUnknown:
		return false
	default:
		return false
	}
}

func CategorizeSimulationError(message string) SimulationErrorType {
	if message == "" {
		return SimulationErrorTypeUnknown
	}

	msgLower := strings.ToLower(message)

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

func isTransactionInvalidError(msgLower string) bool {
	return utils.ContainsAny(msgLower, "unmarshal", "parse", "decode", "invalid transaction")
}

func isAuthError(msgLower string) bool {
	return utils.ContainsAny(msgLower, "authorization", "signature", "unauthorized")
}

func isContractExecutionError(msgLower string) bool {
	return utils.ContainsAny(msgLower,
		"contract execution failed",
		"contract error",
		"contract panic",
		"hosterror: error(storage,",
		"contract already exists",
		"wasm does not exist",
		"existingvalue)",
		"missingvalue)")
}

func isResourceError(msgLower string) bool {
	return utils.ContainsAny(msgLower, "resource", "cpu limit", "memory limit", "instructions limit", "limit exceeded")
}

// RPCClient is an interface that defines the methods for interacting with Stellar RPC.
//
//go:generate mockery --name=RPCClient --case=underscore --structname=MockRPCClient --filename=rpc_client.go
type RPCClient interface {
	SimulateTransaction(ctx context.Context, request protocol.SimulateTransactionRequest) (*SimulationResult, *SimulationError)
}
