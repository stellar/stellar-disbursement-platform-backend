package stellar

import (
	"errors"
	"testing"

	"github.com/stellar/stellar-rpc/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SimulationError_Error(t *testing.T) {
	testCases := []struct {
		name        string
		errorType   SimulationErrorType
		message     string
		originalErr error
		expected    string
	}{
		{
			name:        "error with original error",
			errorType:   SimulationErrorTypeContractExecution,
			message:     "contract execution failed",
			originalErr: errors.New("original error"),
			expected:    "simulation contract_execution error, contract execution failed (original: original error)",
		},
		{
			name:        "error without original error",
			errorType:   SimulationErrorTypeNetwork,
			message:     "network timeout",
			originalErr: nil,
			expected:    "simulation network error, network timeout",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := &SimulationError{
				Type:        tc.errorType,
				Message:     tc.message,
				OriginalErr: tc.originalErr,
			}

			assert.Equal(t, tc.expected, err.Error())
		})
	}
}

func Test_SimulationError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	simErr := &SimulationError{
		Type:        SimulationErrorTypeAuth,
		Message:     "auth failed",
		OriginalErr: originalErr,
	}

	assert.Equal(t, originalErr, simErr.Unwrap())
}

func Test_SimulationError_Unwrap_WithNilOriginal(t *testing.T) {
	simErr := &SimulationError{
		Type:        SimulationErrorTypeAuth,
		Message:     "auth failed",
		OriginalErr: nil,
	}

	assert.Nil(t, simErr.Unwrap())
}

func Test_NewSimulationError(t *testing.T) {
	testCases := []struct {
		name              string
		errorType         SimulationErrorType
		message           string
		originalErr       error
		response          *protocol.SimulateTransactionResponse
		expectedRetryable bool
	}{
		{
			name:              "network error - retryable",
			errorType:         SimulationErrorTypeNetwork,
			message:           "connection failed",
			originalErr:       errors.New("tcp timeout"),
			response:          nil,
			expectedRetryable: true,
		},
		{
			name:              "resource error - retryable",
			errorType:         SimulationErrorTypeResource,
			message:           "cpu limit exceeded",
			originalErr:       nil,
			response:          &protocol.SimulateTransactionResponse{},
			expectedRetryable: true,
		},
		{
			name:              "transaction invalid - not retryable",
			errorType:         SimulationErrorTypeTransactionInvalid,
			message:           "malformed XDR",
			originalErr:       errors.New("xdr parse error"),
			response:          nil,
			expectedRetryable: false,
		},
		{
			name:              "auth error - not retryable",
			errorType:         SimulationErrorTypeAuth,
			message:           "unauthorized",
			originalErr:       nil,
			response:          nil,
			expectedRetryable: false,
		},
		{
			name:              "contract execution error - not retryable",
			errorType:         SimulationErrorTypeContractExecution,
			message:           "contract panic",
			originalErr:       nil,
			response:          nil,
			expectedRetryable: false,
		},
		{
			name:              "unknown error - not retryable",
			errorType:         SimulationErrorTypeUnknown,
			message:           "unexpected error",
			originalErr:       nil,
			response:          nil,
			expectedRetryable: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewSimulationError(tc.errorType, tc.message, tc.originalErr, tc.response)

			require.NotNil(t, err)
			assert.Equal(t, tc.errorType, err.Type)
			assert.Equal(t, tc.message, err.Message)
			assert.Equal(t, tc.originalErr, err.OriginalErr)
			assert.Equal(t, tc.response, err.Response)
			assert.Equal(t, tc.expectedRetryable, err.IsRetryable)
		})
	}
}

func Test_SimulationResult(t *testing.T) {
	response := protocol.SimulateTransactionResponse{
		LatestLedger: 123,
	}

	result := &SimulationResult{
		Response: response,
	}

	assert.Equal(t, response, result.Response)
	assert.Equal(t, uint32(123), result.Response.LatestLedger)
}

func Test_CategorizeSimulationError(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected SimulationErrorType
	}{
		{
			name:     "empty message returns unknown",
			message:  "",
			expected: SimulationErrorTypeUnknown,
		},
		{
			name:     "contract execution failed",
			message:  "contract execution failed",
			expected: SimulationErrorTypeContractExecution,
		},
		{
			name:     "contract error",
			message:  "contract error",
			expected: SimulationErrorTypeContractExecution,
		},
		{
			name:     "contract panic",
			message:  "contract panic",
			expected: SimulationErrorTypeContractExecution,
		},
		{
			name:     "hosterror storage error",
			message:  "HostError: Error(Storage, ExistingValue)",
			expected: SimulationErrorTypeContractExecution,
		},
		{
			name:     "contract already exists",
			message:  "contract already exists",
			expected: SimulationErrorTypeContractExecution,
		},
		{
			name:     "wasm does not exist",
			message:  "Wasm does not exist",
			expected: SimulationErrorTypeContractExecution,
		},
		{
			name:     "existingvalue error",
			message:  "Error(Storage, ExistingValue)",
			expected: SimulationErrorTypeContractExecution,
		},
		{
			name:     "missingvalue error",
			message:  "Error(Storage, MissingValue)",
			expected: SimulationErrorTypeContractExecution,
		},
		// Resource errors
		{
			name:     "resource limit",
			message:  "resource limit exceeded",
			expected: SimulationErrorTypeResource,
		},
		{
			name:     "cpu limit",
			message:  "cpu limit exceeded",
			expected: SimulationErrorTypeResource,
		},
		{
			name:     "memory limit",
			message:  "memory limit exceeded",
			expected: SimulationErrorTypeResource,
		},
		{
			name:     "instructions limit",
			message:  "instructions limit exceeded",
			expected: SimulationErrorTypeResource,
		},
		{
			name:     "limit exceeded",
			message:  "limit exceeded",
			expected: SimulationErrorTypeResource,
		},
		// Transaction invalid errors
		{
			name:     "unmarshal error",
			message:  "failed to unmarshal XDR",
			expected: SimulationErrorTypeTransactionInvalid,
		},
		{
			name:     "parse error",
			message:  "failed to parse transaction",
			expected: SimulationErrorTypeTransactionInvalid,
		},
		{
			name:     "decode error",
			message:  "failed to decode",
			expected: SimulationErrorTypeTransactionInvalid,
		},
		{
			name:     "invalid transaction",
			message:  "invalid transaction format",
			expected: SimulationErrorTypeTransactionInvalid,
		},
		// Auth errors
		{
			name:     "authorization error",
			message:  "authorization failed",
			expected: SimulationErrorTypeAuth,
		},
		{
			name:     "signature error",
			message:  "invalid signature",
			expected: SimulationErrorTypeAuth,
		},
		{
			name:     "unauthorized error",
			message:  "unauthorized access",
			expected: SimulationErrorTypeAuth,
		},
		// Unknown errors
		{
			name:     "unknown error type",
			message:  "some random error message",
			expected: SimulationErrorTypeUnknown,
		},
		// Case insensitive testing
		{
			name:     "case insensitive contract error",
			message:  "CONTRACT EXECUTION FAILED",
			expected: SimulationErrorTypeContractExecution,
		},
		{
			name:     "case insensitive auth error",
			message:  "AUTHORIZATION failed",
			expected: SimulationErrorTypeAuth,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CategorizeSimulationError(tc.message)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_IsTransactionInvalidError(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected bool
	}{
		{
			name:     "unmarshal error",
			message:  "failed to unmarshal",
			expected: true,
		},
		{
			name:     "parse error",
			message:  "cannot parse transaction",
			expected: true,
		},
		{
			name:     "decode error",
			message:  "decode failed",
			expected: true,
		},
		{
			name:     "invalid transaction",
			message:  "invalid transaction format",
			expected: true,
		},
		{
			name:     "not a transaction error",
			message:  "contract execution failed",
			expected: false,
		},
		{
			name:     "empty message",
			message:  "",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isTransactionInvalidError(tc.message)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_IsAuthError(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected bool
	}{
		{
			name:     "authorization error",
			message:  "authorization failed",
			expected: true,
		},
		{
			name:     "signature error",
			message:  "invalid signature",
			expected: true,
		},
		{
			name:     "unauthorized error",
			message:  "unauthorized access",
			expected: true,
		},
		{
			name:     "not an auth error",
			message:  "contract execution failed",
			expected: false,
		},
		{
			name:     "empty message",
			message:  "",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isAuthError(tc.message)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_IsContractExecutionError(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected bool
	}{
		{
			name:     "contract execution failed",
			message:  "contract execution failed",
			expected: true,
		},
		{
			name:     "contract error",
			message:  "contract error occurred",
			expected: true,
		},
		{
			name:     "contract panic",
			message:  "contract panic: division by zero",
			expected: true,
		},
		{
			name:     "hosterror storage - lowercase pattern",
			message:  "hosterror: error(storage, existingvalue)",
			expected: true,
		},
		{
			name:     "contract already exists",
			message:  "contract already exists for this address",
			expected: true,
		},
		{
			name:     "wasm does not exist",
			message:  "wasm does not exist in the ledger",
			expected: true,
		},
		{
			name:     "existingvalue error",
			message:  "something failed with existingvalue)",
			expected: true,
		},
		{
			name:     "missingvalue error",
			message:  "something failed with missingvalue)",
			expected: true,
		},
		{
			name:     "not a contract error",
			message:  "network timeout",
			expected: false,
		},
		{
			name:     "empty message",
			message:  "",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isContractExecutionError(tc.message)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_IsResourceError(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected bool
	}{
		{
			name:     "resource error",
			message:  "resource limit exceeded",
			expected: true,
		},
		{
			name:     "cpu limit",
			message:  "cpu limit exceeded",
			expected: true,
		},
		{
			name:     "memory limit",
			message:  "memory limit reached",
			expected: true,
		},
		{
			name:     "instructions limit",
			message:  "instructions limit exceeded",
			expected: true,
		},
		{
			name:     "limit exceeded",
			message:  "limit exceeded during execution",
			expected: true,
		},
		{
			name:     "not a resource error",
			message:  "contract execution failed",
			expected: false,
		},
		{
			name:     "empty message",
			message:  "",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isResourceError(tc.message)
			assert.Equal(t, tc.expected, result)
		})
	}
}
