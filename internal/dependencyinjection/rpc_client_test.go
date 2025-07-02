package dependencyinjection

import (
	"context"
	"errors"
	"testing"

	"github.com/stellar/stellar-rpc/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
)

func Test_RPCClientWrapper_SimulateTransaction(t *testing.T) {
	ctx := context.Background()
	request := protocol.SimulateTransactionRequest{
		Transaction: "valid-transaction",
	}

	t.Run("nil client returns network error", func(t *testing.T) {
		wrapper := &RPCClientWrapper{client: nil}

		result, simErr := wrapper.SimulateTransaction(ctx, request)

		assert.Nil(t, result)
		require.NotNil(t, simErr)
		assert.Equal(t, stellar.SimulationErrorTypeNetwork, simErr.Type)
		assert.Equal(t, "RPC client not initialized", simErr.Message)
		assert.Nil(t, simErr.OriginalErr)
		assert.True(t, simErr.IsRetryable)
	})

	t.Run("successful simulation", func(t *testing.T) {
		expectedResponse := protocol.SimulateTransactionResponse{
			LatestLedger: 123,
			Error:        "",
		}

		result := &stellar.SimulationResult{
			Response: expectedResponse,
		}

		assert.NotNil(t, result)
		assert.Equal(t, expectedResponse, result.Response)
		assert.Equal(t, uint32(123), result.Response.LatestLedger)
		assert.Equal(t, "", result.Response.Error)
	})

	t.Run("simulation error categorization", func(t *testing.T) {
		testCases := []struct {
			name         string
			errorMessage string
			expectedType stellar.SimulationErrorType
		}{
			{
				name:         "contract execution error",
				errorMessage: "contract execution failed",
				expectedType: stellar.SimulationErrorTypeContractExecution,
			},
			{
				name:         "resource error",
				errorMessage: "cpu limit exceeded",
				expectedType: stellar.SimulationErrorTypeResource,
			},
			{
				name:         "auth error",
				errorMessage: "authorization failed",
				expectedType: stellar.SimulationErrorTypeAuth,
			},
			{
				name:         "transaction invalid error",
				errorMessage: "failed to unmarshal XDR",
				expectedType: stellar.SimulationErrorTypeTransactionInvalid,
			},
			{
				name:         "unknown error",
				errorMessage: "some unknown error",
				expectedType: stellar.SimulationErrorTypeUnknown,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				errorType := stellar.CategorizeSimulationError(tc.errorMessage)
				assert.Equal(t, tc.expectedType, errorType)

				simErr := stellar.NewSimulationError(
					errorType,
					tc.errorMessage,
					nil,
					nil,
				)

				assert.Equal(t, tc.expectedType, simErr.Type)
				assert.Equal(t, tc.errorMessage, simErr.Message)
			})
		}
	})
}

func Test_NewRpcClient(t *testing.T) {
	ctx := context.Background()
	opts := stellar.RPCOptions{
		RPCUrl: "http://localhost:8000/soroban/rpc",
	}

	t.Run("creates new client when none exists", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		client, err := NewRpcClient(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, client)
	})

	t.Run("returns existing client when already created", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		client1, err := NewRpcClient(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, client1)

		client2, err := NewRpcClient(ctx, opts)
		require.NoError(t, err)
		require.NotNil(t, client2)

		assert.Equal(t, client1, client2)
	})

	t.Run("handles auth headers correctly", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		optsWithAuth := stellar.RPCOptions{
			RPCUrl:                    "http://localhost:8000/soroban/rpc",
			RPCRequestAuthHeaderKey:   "Authorization",
			RPCRequestAuthHeaderValue: "Bearer test-token",
		}

		client, err := NewRpcClient(ctx, optsWithAuth)
		require.NoError(t, err)
		require.NotNil(t, client)
	})
}

func Test_RPCClientWrapper_ErrorHandling(t *testing.T) {
	t.Run("network error from underlying client", func(t *testing.T) {
		expectedErr := errors.New("connection timeout")
		simErr := stellar.NewSimulationError(
			stellar.SimulationErrorTypeNetwork,
			expectedErr.Error(),
			expectedErr,
			nil,
		)

		assert.Equal(t, stellar.SimulationErrorTypeNetwork, simErr.Type)
		assert.Equal(t, "connection timeout", simErr.Message)
		assert.Equal(t, expectedErr, simErr.OriginalErr)
		assert.True(t, simErr.IsRetryable)
	})

	t.Run("simulation error with response", func(t *testing.T) {
		errorMessage := "contract execution failed"
		response := &protocol.SimulateTransactionResponse{
			Error:        errorMessage,
			LatestLedger: 456,
		}

		simErr := stellar.NewSimulationError(
			stellar.CategorizeSimulationError(errorMessage),
			errorMessage,
			nil,
			response,
		)

		assert.Equal(t, stellar.SimulationErrorTypeContractExecution, simErr.Type)
		assert.Equal(t, errorMessage, simErr.Message)
		assert.Equal(t, response, simErr.Response)
		assert.False(t, simErr.IsRetryable)
	})
}
