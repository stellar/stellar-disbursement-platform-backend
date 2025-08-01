package stellar

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stellar/stellar-rpc/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewRPCClientWrapper(t *testing.T) {
	t.Run("creates client with default HTTP client", func(t *testing.T) {
		wrapper := NewRPCClientWrapper("https://soroban-testnet.stellar.org", http.DefaultClient)
		require.NotNil(t, wrapper)
		assert.NotNil(t, wrapper.client)
	})

	t.Run("creates client with custom HTTP client", func(t *testing.T) {
		customClient := &http.Client{}
		wrapper := NewRPCClientWrapper("https://soroban-testnet.stellar.org", customClient)
		require.NotNil(t, wrapper)
		assert.NotNil(t, wrapper.client)
	})
}

func Test_RPCClientWrapper_SimulateTransaction(t *testing.T) {
	t.Run("returns error when client is nil", func(t *testing.T) {
		wrapper := &RPCClientWrapper{client: nil}
		ctx := context.Background()
		request := protocol.SimulateTransactionRequest{}

		result, err := wrapper.SimulateTransaction(ctx, request)

		assert.Nil(t, result)
		require.NotNil(t, err)
		assert.Equal(t, SimulationErrorTypeNetwork, err.Type)
		assert.NotNil(t, err.Unwrap())
		assert.Equal(t, "RPC client not initialized", err.Unwrap().Error())
	})

	t.Run("creates proper error chain for simulation errors", func(t *testing.T) {
		// Test that simulation errors from resp.Error create proper error chains
		errorMessage := "contract execution failed"

		simErr := NewSimulationError(
			errors.New(errorMessage),
			&protocol.SimulateTransactionResponse{Error: errorMessage},
		)

		// Verify the error chain is constructed correctly
		assert.Equal(t, SimulationErrorTypeContractExecution, simErr.Type)
		assert.NotNil(t, simErr.Unwrap())
		assert.Equal(t, errorMessage, simErr.Unwrap().Error())
	})
}

func Test_NewHTTPClientWithAuth(t *testing.T) {
	t.Run("returns default client when no auth provided", func(t *testing.T) {
		client, err := NewHTTPClientWithAuth("", "")
		require.NoError(t, err)
		assert.Equal(t, http.DefaultClient, client)
	})

	t.Run("returns error when only key provided", func(t *testing.T) {
		client, err := NewHTTPClientWithAuth("Authorization", "")
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "both authHeaderKey and authHeaderValue must be provided or both must be empty")
	})

	t.Run("returns error when only value provided", func(t *testing.T) {
		client, err := NewHTTPClientWithAuth("", "Bearer token")
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "both authHeaderKey and authHeaderValue must be provided or both must be empty")
	})

	t.Run("returns custom client when both key and value provided", func(t *testing.T) {
		client, err := NewHTTPClientWithAuth("Authorization", "Bearer token")
		require.NoError(t, err)
		assert.NotEqual(t, http.DefaultClient, client)
		assert.NotNil(t, client.Transport)
	})
}
