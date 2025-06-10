package dependencyinjection

import (
	"context"
	"fmt"
	"net/http"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/stellar"
	"github.com/stellar/stellar-rpc/client"
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

	httpClient := http.DefaultClient
	if opts.RPCRequestHeaderKey != "" && opts.RPCRequestHeaderValue != "" {
		transport := &headerTransport{
			base:  http.DefaultTransport,
			key:   opts.RPCRequestHeaderKey,
			value: opts.RPCRequestHeaderValue,
		}
		httpClient = &http.Client{
			Transport: transport,
		}
	}

	rpcClient := client.NewClient(opts.RPCUrl, httpClient)

	return rpcClient, nil
}
