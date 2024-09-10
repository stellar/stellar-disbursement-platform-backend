package sorobanrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Authenticator defines an interface for injecting authentication into a request.
type Authenticator interface {
	Authenticate(req *http.Request)
}

// BearerTokenAuthenticator implements the Authenticator interface for Bearer token authentication.
type BearerTokenAuthenticator struct {
	Token string
}

// Authenticate injects the Bearer token into the request header.
func (b *BearerTokenAuthenticator) Authenticate(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+b.Token)
}

// Client represents a Soroban RPC client.
type Client struct {
	endpoint      string
	client        *http.Client
	authenticator Authenticator
}

// NewClient creates a new Soroban RPC client with the specified RPC server endpoint and optional authenticator.
func NewClient(endpoint string, auth Authenticator) *Client {
	return &Client{
		endpoint:      endpoint,
		client:        http.DefaultClient,
		authenticator: auth,
	}
}

// RPCRequest represents the JSON-RPC request structure.
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params,omitempty"`
	ID      int           `json:"id"`
}

// RPCResponse represents the JSON-RPC response structure.
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// RPCError represents an error in the JSON-RPC response.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Reserved error codes in JSON-RPC 2.0 specification. Link: https://www.jsonrpc.org/specification#error_object.
const (
	ParseError     = -32700 // Invalid JSON received by the server
	InvalidRequest = -32600 // The JSON sent is not a valid request object
	MethodNotFound = -32601 // The method does not exist or is not available
	InvalidParams  = -32602 // Invalid method parameter(s)
	InternalError  = -32603 // Internal JSON-RPC error
	// -32000 to -32099	Server error	Reserved for implementation-defined server-errors.
)

// Call sends a JSON-RPC request to the Soroban RPC server, using variadic params.
func (c *Client) Call(ctx context.Context, method string, id int, params ...interface{}) (*RPCResponse, error) {
	// Create the request object
	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      id,
	}

	// Serialize the request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create the HTTP request
	httpReq, err := http.NewRequest("POST", c.endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Authenticate the request if an authenticator is provided
	if c.authenticator != nil {
		c.authenticator.Authenticate(httpReq)
	}

	// Send the request to the Soroban RPC server
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Decode the JSON-RPC response
	var rpcResp RPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for any RPC errors
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: code=%d, message=%s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return &rpcResp, nil
}
