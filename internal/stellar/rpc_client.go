package stellar

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/stellar-rpc/client"
	"github.com/stellar/stellar-rpc/protocol"
)

type RPCClientWrapper struct {
	client *client.Client
}

func NewRPCClientWrapper(rpcUrl string, httpClient *http.Client) *RPCClientWrapper {
	innerClient := client.NewClient(rpcUrl, httpClient)
	return &RPCClientWrapper{client: innerClient}
}

func (w *RPCClientWrapper) SimulateTransaction(ctx context.Context, request protocol.SimulateTransactionRequest) (*SimulationResult, *SimulationError) {
	if w.client == nil {
		return nil, NewSimulationError(
			errors.New("RPC client not initialized"),
			nil,
		)
	}

	resp, err := w.client.SimulateTransaction(ctx, request)

	if err != nil {
		return nil, NewSimulationError(
			err,
			nil,
		)
	}

	if resp.Error != "" {
		return nil, NewSimulationError(
			errors.New(resp.Error),
			&resp,
		)
	}

	return &SimulationResult{
		Response: resp,
	}, nil
}

func NewHTTPClientWithAuth(authHeaderKey, authHeaderValue string) (*http.Client, error) {
	if authHeaderKey == "" && authHeaderValue == "" {
		return http.DefaultClient, nil
	}

	if authHeaderKey == "" || authHeaderValue == "" {
		return nil, fmt.Errorf("both authHeaderKey and authHeaderValue must be provided or both must be empty")
	}

	transport := &headerTransport{
		base:  http.DefaultTransport,
		key:   authHeaderKey,
		value: authHeaderValue,
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

type headerTransport struct {
	base  http.RoundTripper
	key   string
	value string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add(t.key, t.value)
	return t.base.RoundTrip(req)
}

var _ RPCClient = (*RPCClientWrapper)(nil)
