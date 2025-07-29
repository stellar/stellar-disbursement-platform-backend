package dependencyinjection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
)

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

	t.Run("returns error when only auth header key provided", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		optsWithIncompleteAuth := stellar.RPCOptions{
			RPCUrl:                  "http://localhost:8000/soroban/rpc",
			RPCRequestAuthHeaderKey: "Authorization",
		}

		client, err := NewRpcClient(ctx, optsWithIncompleteAuth)
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "error creating HTTP client")
	})

	t.Run("returns error when only auth header value provided", func(t *testing.T) {
		ClearInstancesTestHelper(t)

		optsWithIncompleteAuth := stellar.RPCOptions{
			RPCUrl:                    "http://localhost:8000/soroban/rpc",
			RPCRequestAuthHeaderValue: "Bearer test-token",
		}

		client, err := NewRpcClient(ctx, optsWithIncompleteAuth)
		require.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "error creating HTTP client")
	})
}
