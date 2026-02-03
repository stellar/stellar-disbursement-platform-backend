package stellar

import (
	"errors"
	"testing"

	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SimulationError_Error(t *testing.T) {
	testCases := []struct {
		name        string
		errorType   SimulationErrorType
		originalErr error
		expected    string
	}{
		{
			name:        "error with original error",
			errorType:   SimulationErrorTypeContractExecution,
			originalErr: errors.New("original error"),
			expected:    "simulation contract_execution error: original error",
		},
		{
			name:        "error without original error",
			errorType:   SimulationErrorTypeNetwork,
			originalErr: nil,
			expected:    "simulation network error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := &SimulationError{
				Type: tc.errorType,
				Err:  tc.originalErr,
			}

			assert.Equal(t, tc.expected, err.Error())
		})
	}
}

func Test_SimulationError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	simErr := &SimulationError{
		Type: SimulationErrorTypeAuth,
		Err:  originalErr,
	}

	assert.Equal(t, originalErr, simErr.Unwrap())
}

func Test_SimulationError_Unwrap_WithNilOriginal(t *testing.T) {
	simErr := &SimulationError{
		Type: SimulationErrorTypeAuth,
		Err:  nil,
	}

	assert.Nil(t, simErr.Unwrap())
}

func Test_NewSimulationError(t *testing.T) {
	testCases := []struct {
		name              string
		originalErr       error
		response          *protocol.SimulateTransactionResponse
		expectedType      SimulationErrorType
		expectedRetryable bool
	}{
		{
			name:              "network error - retryable",
			originalErr:       errors.New("tcp timeout"),
			response:          nil,
			expectedType:      SimulationErrorTypeNetwork,
			expectedRetryable: true,
		},
		{
			name:              "resource error - retryable",
			originalErr:       errors.New("resource limit exceeded"),
			response:          &protocol.SimulateTransactionResponse{Error: "resource limit exceeded"},
			expectedType:      SimulationErrorTypeResource,
			expectedRetryable: true,
		},
		{
			name:              "transaction invalid - not retryable",
			originalErr:       errors.New("xdr parse error"),
			response:          &protocol.SimulateTransactionResponse{Error: "xdr parse error"},
			expectedType:      SimulationErrorTypeTransactionInvalid,
			expectedRetryable: false,
		},
		{
			name:              "auth error - not retryable",
			originalErr:       errors.New("authorization failed"),
			response:          &protocol.SimulateTransactionResponse{Error: "authorization failed"},
			expectedType:      SimulationErrorTypeAuth,
			expectedRetryable: false,
		},
		{
			name:              "contract execution error - not retryable",
			originalErr:       errors.New("contract execution failed"),
			response:          &protocol.SimulateTransactionResponse{Error: "contract execution failed"},
			expectedType:      SimulationErrorTypeContractExecution,
			expectedRetryable: false,
		},
		{
			name:              "unknown error - not retryable",
			originalErr:       errors.New("some random error"),
			response:          &protocol.SimulateTransactionResponse{Error: "some random error"},
			expectedType:      SimulationErrorTypeUnknown,
			expectedRetryable: false,
		},
		{
			name:              "nil error - unknown type",
			originalErr:       nil,
			response:          nil,
			expectedType:      SimulationErrorTypeUnknown,
			expectedRetryable: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewSimulationError(tc.originalErr, tc.response)

			require.NotNil(t, err)
			assert.Equal(t, tc.expectedType, err.Type)
			assert.Equal(t, tc.originalErr, err.Unwrap())
			assert.Equal(t, tc.response, err.Response)
			assert.Equal(t, tc.expectedRetryable, err.IsRetryable())
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
