package dependencyinjection

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-rpc/client"
	"github.com/stellar/stellar-rpc/protocol"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
)

const RpcClientInstanceName = "rpc_client_instance"

type headerTransport struct {
	base  http.RoundTripper
	key   string
	value string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add(t.key, t.value)
	return t.base.RoundTrip(req)
}

type RPCClientWrapper struct {
	client *client.Client
}

func (w *RPCClientWrapper) SimulateTransaction(ctx context.Context, request protocol.SimulateTransactionRequest) (*stellar.SimulationResult, *stellar.SimulationError) {
	if w.client == nil {
		return nil, stellar.NewSimulationError(
			stellar.SimulationErrorTypeNetwork,
			"RPC client not initialized",
			nil,
			nil,
		)
	}

	resp, err := w.client.SimulateTransaction(ctx, request)

	if err != nil {
		return nil, stellar.NewSimulationError(
			stellar.SimulationErrorTypeNetwork,
			err.Error(),
			err,
			nil,
		)
	}

	if resp.Error != "" {
		errorType := stellar.CategorizeSimulationError(resp.Error)
		return nil, stellar.NewSimulationError(
			errorType,
			resp.Error,
			nil,
			&resp,
		)
	}

	return &stellar.SimulationResult{
		Response: resp,
	}, nil
}

func NewRpcClient(ctx context.Context, opts stellar.RPCOptions) (stellar.RPCClient, error) {
	if instance, ok := GetInstance(RpcClientInstanceName); ok {
		if rpcClient, ok := instance.(stellar.RPCClient); ok {
			return rpcClient, nil
		}
		return nil, fmt.Errorf("error trying to cast rpc client instance")
	}

	log.Ctx(ctx).Info("⚙️ Setting up RPC Client")

	httpClient := http.DefaultClient
	if opts.RPCRequestAuthHeaderKey != "" && opts.RPCRequestAuthHeaderValue != "" {
		transport := &headerTransport{
			base:  http.DefaultTransport,
			key:   opts.RPCRequestAuthHeaderKey,
			value: opts.RPCRequestAuthHeaderValue,
		}
		httpClient = &http.Client{
			Transport: transport,
		}
	}

	innerClient := client.NewClient(opts.RPCUrl, httpClient)
	rpcClient := &RPCClientWrapper{client: innerClient}

	SetInstance(RpcClientInstanceName, rpcClient)

	return rpcClient, nil
}
