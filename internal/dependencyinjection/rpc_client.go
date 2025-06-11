package dependencyinjection

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-rpc/client"

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

func NewRpcClient(ctx context.Context, opts stellar.RPCOptions) (*client.Client, error) {
	if instance, ok := GetInstance(RpcClientInstanceName); ok {
		if rpcClient, ok := instance.(*client.Client); ok {
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

	rpcClient := client.NewClient(opts.RPCUrl, httpClient)

	SetInstance(RpcClientInstanceName, rpcClient)

	return rpcClient, nil
}
