package sorobanrpc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/stellar/go/xdr"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
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
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      int         `json:"id"`
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
func (c *Client) Call(ctx context.Context, id int, method string, params interface{}) (*RPCResponse, error) {
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

//////// Get Health

type HealthStatus string

const HealthStatusHealthy HealthStatus = "healthy"

// {"status":"healthy","latestLedger":1513174,"oldestLedger":1495895,"ledgerRetentionWindow":17280}
type GetHealthResult struct {
	Status                HealthStatus `json:"status"`
	OldestLedger          int          `json:"oldestLedger"`
	LatestLedger          int          `json:"latestLedger"`
	LedgerRetentionWindow int          `json:"ledgerRetentionWindow"`
}

type GetHealthResponse struct {
	RPCResponse
	Result GetHealthResult `json:"result,omitempty"`
}

func (c *Client) GetHealth(ctx context.Context, id int) (*GetHealthResponse, error) {
	rpcResp, err := c.Call(ctx, id, "getHealth", nil)
	if err != nil {
		return nil, fmt.Errorf("calling RPC server with getHealth: %w", err)
	}

	getHealthResp, err := utils.ConvertType[*RPCResponse, *GetHealthResponse](rpcResp)
	if err != nil {
		return nil, fmt.Errorf("converting RPC response to GetHealthResponse: %w", err)
	}

	return getHealthResp, nil
}

//////// Get Transaction

type TransactionStatus string

const (
	TransactionStatusSuccess  TransactionStatus = "SUCCESS"
	TransactionStatusFailed   TransactionStatus = "FAILED"
	TransactionStatusNotFound TransactionStatus = "NOT_FOUND"
)

type GetTransactionResult struct {
	Status                TransactionStatus `json:"status"`
	LatestLedger          int               `json:"latestLedger"`
	LatestLedgerCloseTime string            `json:"latestLedgerCloseTime"`
	OldestLedger          int               `json:"oldestLedger"`
	OldestLedgerCloseTime string            `json:"oldestLedgerCloseTime"`
	ApplicationOrder      int               `json:"applicationOrder,omitempty"`
	EnvelopeXdr           string            `json:"envelopeXdr,omitempty"`
	ResultXdr             string            `json:"resultXdr,omitempty"`
	ResultMetaXdr         string            `json:"resultMetaXdr,omitempty"`
	Ledger                int               `json:"ledger,omitempty"`
	CreatedAt             string            `json:"createdAt,omitempty"`
}

type GetTransactionResponse struct {
	RPCResponse
	Result GetTransactionResult `json:"result,omitempty"`
}

func (c *Client) GetTransaction(ctx context.Context, id int, hash string) (*GetTransactionResponse, error) {
	rpcResp, err := c.Call(ctx, id, "getTransaction", map[string]string{"hash": hash})
	if err != nil {
		return nil, fmt.Errorf("calling RPC server with getTransaction: %w", err)
	}

	getTransactionResp, err := utils.ConvertType[*RPCResponse, *GetTransactionResponse](rpcResp)
	if err != nil {
		return nil, fmt.Errorf("converting RPC response to GetTransactionResponse: %w", err)
	}

	return getTransactionResp, nil
}

//////// Send Transaction

type SendTransactionStatus string

const (
	SendTransactionStatusPending       SendTransactionStatus = "PENDING"
	SendTransactionStatusDuplicate     SendTransactionStatus = "DUPLICATE"
	SendTransactionStatusTryAgainLater SendTransactionStatus = "TRY_AGAIN_LATER"
	SendTransactionStatusError         SendTransactionStatus = "ERROR"
)

type ErrorResultXDR xdr.TransactionResult

// UnmarshalJSON implements the json.Unmarshaler interface for ErrorResultXDR
func (e *ErrorResultXDR) UnmarshalJSON(data []byte) error {
	// Step 1: Parse the JSON input to get the base64-encoded XDR string
	var base64Xdr string
	if err := json.Unmarshal(data, &base64Xdr); err != nil {
		return fmt.Errorf("error unmarshalling JSON into base64 string: %w", err)
	}

	// Step 2: Decode the base64-encoded XDR string
	rawXdr, err := base64.StdEncoding.DecodeString(base64Xdr)
	if err != nil {
		return fmt.Errorf("error decoding base64 XDR string: %w", err)
	}

	// Step 3: Unmarshal the raw XDR bytes into an xdr.TransactionResult object
	var txResult xdr.TransactionResult
	err = xdr.SafeUnmarshal(rawXdr, &txResult)
	if err != nil {
		return fmt.Errorf("error unmarshalling XDR to TransactionResult: %w", err)
	}

	// Step 4: Assign the unmarshalled TransactionResult to the receiver
	*e = ErrorResultXDR(txResult)

	return nil
}

type SendTransactionResult struct {
	Hash                  string                `json:"hash"`
	Status                SendTransactionStatus `json:"status"`
	LatestLedger          int                   `json:"latestLedger"`
	LatestLedgerCloseTime string                `json:"latestLedgerCloseTime"`
	DiagnosticEventsXDR   string                `json:"diagnosticEventsXdr,omitempty"`
	// TODO: maybe use xdr.TransactionResult
	ErrorResultXDR ErrorResultXDR `json:"errorResultXdr,omitempty"`
	// ErrorResultXDR string `json:"errorResultXdr,omitempty"`
}

type SendTransactionResponse struct {
	RPCResponse
	Result SendTransactionResult `json:"result,omitempty"`
}

func (c *Client) SendTransaction(ctx context.Context, id int, txXDR string) (*SendTransactionResponse, error) {
	rpcResp, err := c.Call(ctx, id, "sendTransaction", map[string]string{"transaction": txXDR})
	if err != nil {
		return nil, fmt.Errorf("calling RPC server with sendTransaction: %w", err)
	}

	sendTransactionResp, err := utils.ConvertType[*RPCResponse, *SendTransactionResponse](rpcResp)
	if err != nil {
		return nil, fmt.Errorf("converting RPC response to SendTransactionResponse: %w", err)
	}

	return sendTransactionResp, nil
}

//////// Simulate Transaction

type SimulateTransactionResult struct {
	TransactionData string   `json:"transactionData"`
	MinResourceFee  string   `json:"minResourceFee"`
	Events          []string `json:"events"`
	Results         []struct {
		Auth []interface{} `json:"auth"`
		XDR  string        `json:"xdr"`
	} `json:"results"`
	Cost struct {
		CPUInsns string `json:"cpuInsns"`
		MemBytes string `json:"memBytes"`
	} `json:"cost"`
	LatestLedger int `json:"latestLedger"`
}

type SimulateTransactionResponse struct {
	RPCResponse
	Result SimulateTransactionResult `json:"result,omitempty"`
}

func (c *Client) SimulateTransaction(ctx context.Context, id int, txXDR string) (*SimulateTransactionResponse, error) {
	rpcResp, err := c.Call(ctx, id, "simulateTransaction", map[string]string{"transaction": txXDR})
	if err != nil {
		return nil, fmt.Errorf("calling RPC server with simulateTransaction: %w", err)
	}

	simulateTransactionResp, err := utils.ConvertType[*RPCResponse, *SimulateTransactionResponse](rpcResp)
	if err != nil {
		return nil, fmt.Errorf("converting RPC response to SimulateTransactionResponse: %w", err)
	}

	return simulateTransactionResp, nil
}
