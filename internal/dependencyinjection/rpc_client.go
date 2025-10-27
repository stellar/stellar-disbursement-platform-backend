package dependencyinjection

import (
	"context"
	"fmt"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
)

const RPCClientInstanceName = "rpc_client_instance"

func NewRPCClient(ctx context.Context, opts stellar.RPCOptions) (stellar.RPCClient, error) {
	if instance, ok := GetInstance(RPCClientInstanceName); ok {
		if rpcClient, ok := instance.(stellar.RPCClient); ok {
			return rpcClient, nil
		}
		return nil, fmt.Errorf("error trying to cast rpc client instance")
	}

	log.Ctx(ctx).Info("⚙️ Setting up RPC Client")

	httpClient, err := stellar.NewHTTPClientWithAuth(opts.RPCRequestAuthHeaderKey, opts.RPCRequestAuthHeaderValue)
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	rpcClient := stellar.NewRPCClientWrapper(opts.RPCUrl, httpClient)

	SetInstance(RPCClientInstanceName, rpcClient)

	return rpcClient, nil
}
