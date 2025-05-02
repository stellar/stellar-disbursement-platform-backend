package stellar

type RpcOptions struct {
	// URL of the Stellar RPC server where this application will communicate with.
	RpcURL string
	// The key of the request header to be used for authentication with the RPC server.
	RpcRequestHeaderKey string
	// The value of the request header to be used for authentication with the RPC server.
	RpcRequestHeaderValue string
}
