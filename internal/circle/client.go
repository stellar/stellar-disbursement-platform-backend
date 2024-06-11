package circle

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
	pingPath     = "/ping"
	transferPath = "/v1/transfers"
)

// ClientInterface defines the interface for interacting with the Circle API.
type ClientInterface interface {
	Ping(ctx context.Context) (bool, error)
	PostTransfer(ctx context.Context, transferRequest TransferRequest) (*Transfer, error)
	GetTransferByID(ctx context.Context, id string) (*Transfer, error)
}

// Client provides methods to interact with the Circle API.
type Client struct {
	BasePath   string
	APIKey     string
	httpClient httpclient.HttpClientInterface
}

// NewClient creates a new instance of Circle Client.
func NewClient(env Environment, apiKey string) *Client {
	return &Client{
		BasePath:   string(env),
		APIKey:     apiKey,
		httpClient: httpclient.DefaultClient(),
	}
}

// Ping checks that the service is running.
// https://developers.circle.com/circle-mint/reference/ping.
func (client *Client) Ping(ctx context.Context) (bool, error) {
	u, err := url.JoinPath(client.BasePath, pingPath)
	if err != nil {
		return false, fmt.Errorf("building path: %w", err)
	}

	resp, err := client.request(ctx, u, http.MethodGet, false, nil)
	if err != nil {
		return false, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var pingResp struct {
		Message string `json:"message"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&pingResp); err != nil {
		return false, err
	}

	if pingResp.Message == "pong" {
		return true, nil
	}

	return false, fmt.Errorf("unexpected response message: %s", pingResp.Message)
}

// PostTransfer creates a new transfer.
// https://developers.circle.com/circle-mint/reference/createbusinesstransfer.
func (client *Client) PostTransfer(ctx context.Context, transferReq TransferRequest) (*Transfer, error) {
	err := transferReq.validate()
	if err != nil {
		return nil, fmt.Errorf("validating transfer request: %w", err)
	}

	u, err := url.JoinPath(client.BasePath, transferPath)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	transferReq.IdempotencyKey = uuid.NewString()
	transferData, err := json.Marshal(transferReq)
	if err != nil {
		return nil, err
	}

	resp, err := client.request(ctx, u, http.MethodPost, true, bytes.NewBuffer(transferData))
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		apiError, parseErr := parseAPIError(resp)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing API error: %w", parseErr)
		}
		return nil, fmt.Errorf("API error: %w", apiError)
	}

	return parseTransferResponse(resp)
}

// GetTransferByID retrieves a transfer by its ID.
// https://developers.circle.com/circle-mint/reference/getbusinesstransfer
func (client *Client) GetTransferByID(ctx context.Context, id string) (*Transfer, error) {
	u, err := url.JoinPath(client.BasePath, transferPath, id)
	if err != nil {
		return nil, fmt.Errorf("building path: %w", err)
	}

	resp, err := client.request(ctx, u, http.MethodGet, true, nil)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		apiError, parseErr := parseAPIError(resp)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing API error: %w", parseErr)
		}
		return nil, fmt.Errorf("API error: %w", apiError)
	}

	return parseTransferResponse(resp)
}

// request makes an HTTP request to the Circle API.
func (client *Client) request(ctx context.Context, u string, method string, isAuthed bool, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}

	if isAuthed {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.APIKey))
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return client.httpClient.Do(req)
}

var _ ClientInterface = (*Client)(nil)
