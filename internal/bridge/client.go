package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

const (
	kycLinksPath        = "/v0/kyc_links"
	virtualAccountsPath = "/v0/customers/%s/virtual_accounts"
)

// BridgeErrorResponse represents an error response from the Bridge API.
type BridgeErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Details string `json:"details,omitempty"`
	Source  struct {
		Location string            `json:"location"`
		Key      map[string]string `json:"key,omitempty"`
	}
}

func (e BridgeErrorResponse) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("Bridge API error [%s] = %s - %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("Bridge API error [%s] = %s", e.Code, e.Message)
}

// ClientInterface defines the interface for interacting with the Bridge API.
//
//go:generate mockery --name=ClientInterface --case=underscore --structname=MockClient --filename=client_mock.go --inpackage
type ClientInterface interface {
	PostKYCLink(ctx context.Context, request KYCLinkRequest) (*KYCLinkInfo, error)
	GetKYCLink(ctx context.Context, kycLinkID string) (*KYCLinkInfo, error)
	PostVirtualAccount(ctx context.Context, customerID string, request VirtualAccountRequest) (*VirtualAccountInfo, error)
	GetVirtualAccount(ctx context.Context, customerID, virtualAccountID string) (*VirtualAccountInfo, error)
}

// Client provides methods to interact with the Bridge API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient httpclient.HttpClientInterface
}

type ClientOptions struct {
	BaseURL string
	APIKey  string
}

// Validate validates the ClientOptions fields.
func (opts ClientOptions) Validate() error {
	if opts.BaseURL == "" {
		return fmt.Errorf("baseURL is required")
	}
	if opts.APIKey == "" {
		return fmt.Errorf("apiKey is required")
	}
	return nil
}

// NewClient creates a new instance of Bridge Client.
func NewClient(opts ClientOptions) (ClientInterface, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating client options: %w", err)
	}
	return &Client{
		baseURL:    opts.BaseURL,
		apiKey:     opts.APIKey,
		httpClient: httpclient.DefaultClient(),
	}, nil
}

// PostKYCLink creates a new KYC verification link.
func (c *Client) PostKYCLink(ctx context.Context, request KYCLinkRequest) (*KYCLinkInfo, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("validating KYC link request: %w", err)
	}

	u, err := url.JoinPath(c.baseURL, kycLinksPath)
	if err != nil {
		return nil, fmt.Errorf("building URL path: %w", err)
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.makeRequest(ctx, http.MethodPost, u, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Parse successful response
	var kycResponse KYCLinkInfo
	if jsonErr := json.NewDecoder(resp.Body).Decode(&kycResponse); jsonErr != nil {
		return nil, fmt.Errorf("decoding success response: %w", jsonErr)
	}

	return &kycResponse, nil
}

// GetKYCLink retrieves the current status of a KYC link.
func (c *Client) GetKYCLink(ctx context.Context, kycLinkID string) (*KYCLinkInfo, error) {
	if kycLinkID == "" {
		return nil, fmt.Errorf("kycLinkID is required")
	}

	u, err := url.JoinPath(c.baseURL, kycLinksPath, kycLinkID)
	if err != nil {
		return nil, fmt.Errorf("building URL path: %w", err)
	}

	resp, err := c.makeRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Parse successful response
	var kycResponse KYCLinkInfo
	if jsonErr := json.NewDecoder(resp.Body).Decode(&kycResponse); jsonErr != nil {
		return nil, fmt.Errorf("decoding success response: %w", jsonErr)
	}

	return &kycResponse, nil
}

// PostVirtualAccount creates a new virtual account for a customer
func (c *Client) PostVirtualAccount(ctx context.Context, customerID string, request VirtualAccountRequest) (*VirtualAccountInfo, error) {
	if customerID == "" {
		return nil, fmt.Errorf("customerID is required")
	}

	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("validating virtual account request: %w", err)
	}

	u, err := url.JoinPath(c.baseURL, fmt.Sprintf(virtualAccountsPath, customerID))
	if err != nil {
		return nil, fmt.Errorf("building URL path: %w", err)
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.makeRequest(ctx, http.MethodPost, u, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Parse successful response
	var vaResponse VirtualAccountInfo
	if jsonErr := json.NewDecoder(resp.Body).Decode(&vaResponse); jsonErr != nil {
		return nil, fmt.Errorf("decoding success response: %w", jsonErr)
	}

	return &vaResponse, nil
}

// GetVirtualAccount retrieves a specific virtual account by ID
func (c *Client) GetVirtualAccount(ctx context.Context, customerID, virtualAccountID string) (*VirtualAccountInfo, error) {
	if customerID == "" {
		return nil, fmt.Errorf("customerID is required")
	}

	if virtualAccountID == "" {
		return nil, fmt.Errorf("virtualAccountID is required")
	}

	path := fmt.Sprintf(virtualAccountsPath, customerID)
	u, err := url.JoinPath(c.baseURL, path, virtualAccountID)
	if err != nil {
		return nil, fmt.Errorf("building URL path: %w", err)
	}

	resp, err := c.makeRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Parse successful response
	var vaResponse VirtualAccountInfo
	if jsonErr := json.NewDecoder(resp.Body).Decode(&vaResponse); jsonErr != nil {
		return nil, fmt.Errorf("decoding success response: %w", jsonErr)
	}

	return &vaResponse, nil
}

// makeRequest constructs and sends an HTTP request to the Bridge API.
func (c *Client) makeRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Api-Key", c.apiKey)
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.New().String())
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("making HTTP request: %w", err)
	}

	if err = c.handleErrorResponse(resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// handleErrorResponse processes HTTP error responses and returns appropriate errors.
func (c *Client) handleErrorResponse(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}

	defer resp.Body.Close()

	var bridgeError BridgeErrorResponse
	if jsonErr := json.NewDecoder(resp.Body).Decode(&bridgeError); jsonErr != nil {
		return fmt.Errorf("bridge API returned status %d: %w", resp.StatusCode, jsonErr)
	}
	return bridgeError
}

var _ ClientInterface = (*Client)(nil)
