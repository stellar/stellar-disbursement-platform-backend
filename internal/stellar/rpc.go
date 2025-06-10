package stellar

import (
	"context"

	"github.com/stellar/stellar-rpc/protocol"
)

// RPCOptions contains the configuration options for the Stellar RPC server.
type RPCOptions struct {
	// URL of the Stellar RPC server where this application will communicate with.
	RPCUrl string
	// The key of the request header to be used for authentication with the RPC server.
	RPCRequestHeaderKey string
	// The value of the request header to be used for authentication with the RPC server.
	RPCRequestHeaderValue string
}

// RPCClient is an interface that defines the methods for interacting with Stellar RPC.
type RPCClient interface {
	SimulateTransaction(ctx context.Context, request protocol.SimulateTransactionRequest) (protocol.SimulateTransactionResponse, error)
}
