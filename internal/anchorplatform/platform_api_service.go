package anchorplatform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/schema"
	"golang.org/x/exp/slices"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

var (
	ErrJWTManagerNotSet    = fmt.Errorf("jwt manager not set")
	ErrAuthNotEnforcedOnAP = fmt.Errorf("anchor platform is not enforcing authentication")
	ErrServiceUnavailable  = fmt.Errorf("anchor platform service is unavailable")
)

// TODO update with the PlatformAPI endpoints
type AnchorPlatformAPIServiceInterface interface {
	UpdateAnchorTransactions(ctx context.Context, transactions []Transaction) error
	IsAnchorProtectedByAuth(ctx context.Context) (bool, error)
}

type AnchorPlatformAPIService struct {
	HttpClient                    httpclient.HttpClientInterface
	AnchorPlatformBasePlatformURL string
	jwtManager                    *JWTManager
}

type TransactionValues struct {
	ID                 string `json:"id"`
	Status             string `json:"status,omitempty"`
	Sep                string `json:"sep,omitempty"`
	Kind               string `json:"kind,omitempty"`
	DestinationAccount string `json:"destination_account,omitempty"`
	Memo               string `json:"memo,omitempty"`
	KYCVerified        bool   `json:"kyc_verified,omitempty"`
}

type Transaction struct {
	TransactionValues TransactionValues `json:"transaction"`
}

type TransactionRecords struct {
	Transactions []Transaction `json:"records"`
}

func NewAnchorPlatformAPIService(httpClient httpclient.HttpClientInterface, anchorPlatformBasePlatformURL, anchorPlatformOutgoingJWTSecret string) (*AnchorPlatformAPIService, error) {
	// validation
	if httpClient == nil {
		return nil, fmt.Errorf("http client cannot be nil")
	}
	if anchorPlatformBasePlatformURL == "" {
		return nil, fmt.Errorf("anchor platform base platform url cannot be empty")
	}
	if anchorPlatformOutgoingJWTSecret == "" {
		return nil, fmt.Errorf("anchor platform outgoing jwt secret cannot be empty")
	}

	const expirationMiliseconds = 5000
	jwtManager, err := NewJWTManager(anchorPlatformOutgoingJWTSecret, expirationMiliseconds)
	if err != nil {
		return nil, fmt.Errorf("creating jwt manager: %w", err)
	}

	return &AnchorPlatformAPIService{
		HttpClient:                    httpClient,
		AnchorPlatformBasePlatformURL: anchorPlatformBasePlatformURL,
		jwtManager:                    jwtManager,
	}, nil
}

func (a *AnchorPlatformAPIService) UpdateAnchorTransactions(ctx context.Context, transactions []Transaction) error {
	records := TransactionRecords{transactions}

	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("error marshaling records: %w", err)
	}

	u, err := url.JoinPath(a.AnchorPlatformBasePlatformURL, "transactions")
	if err != nil {
		return fmt.Errorf("error creating url: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, strings.NewReader(string(recordsJSON)))
	if err != nil {
		return fmt.Errorf("error creating new request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	token, err := a.GetJWTToken(transactions)
	if err != nil {
		return fmt.Errorf("getting jwt token in UpdateAnchorTransactions: %w", err)
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	response, err := a.HttpClient.Do(request)
	if err != nil {
		return fmt.Errorf("error making request to anchor platform: %w", err)
	}

	if response.StatusCode/100 != 2 {
		return fmt.Errorf("error updating transaction on anchor platform, response.StatusCode: %d", response.StatusCode)
	}

	return nil
}

type GetTransactionsQueryParams struct {
	SEP        string   `schema:"sep,required,omitempty"`
	Order      string   `schema:"order,omitempty"`
	OrderBy    string   `schema:"order_by,omitempty"`
	PageNumber int      `schema:"page_number,omitempty"`
	PageSize   int      `schema:"page_size,omitempty"`
	Statuses   []string `schema:"statuses,omitempty"`
}

func (a *AnchorPlatformAPIService) getAnchorTransactions(ctx context.Context, skipAuthentication bool, queryParams GetTransactionsQueryParams) (*http.Response, error) {
	// Path
	u, err := url.Parse(a.AnchorPlatformBasePlatformURL)
	if err != nil {
		return nil, fmt.Errorf("creating url to GET anchor transactions: %w", err)
	}
	u = u.JoinPath("transactions")

	// Query parameters
	queryParamsEncoder := schema.NewEncoder()
	params := url.Values{}
	err = queryParamsEncoder.Encode(queryParams, params)
	if err != nil {
		return nil, fmt.Errorf("encoding query params in getAnchorTransactions: %w", err)
	}
	u.RawQuery = params.Encode()

	// request
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request to GET anchor transactions: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	// (skippable) JWT token
	if !skipAuthentication {
		var token string
		token, err = a.GetJWTToken(nil)
		if err != nil {
			return nil, fmt.Errorf("getting jwt token in getAnchorTransactions: %w", err)
		}
		request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	// Do request
	response, err := a.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("making getAnchorTransactions request to anchor platform: %w", err)
	}

	return response, nil
}

func (a *AnchorPlatformAPIService) IsAnchorProtectedByAuth(ctx context.Context) (bool, error) {
	queryParams := GetTransactionsQueryParams{SEP: "24"}
	resp, err := a.getAnchorTransactions(ctx, true, queryParams)
	if err != nil {
		return false, fmt.Errorf("getting anchor transactions from platform API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return false, fmt.Errorf("platform API is returning an unexpected response statusCode=%d: %w", resp.StatusCode, ErrServiceUnavailable)
	}

	if !slices.Contains([]int{http.StatusUnauthorized, http.StatusForbidden}, resp.StatusCode) {
		return false, nil
	}

	return true, nil
}

// GetJWTToken will generate a JWT token if the service is configured with an outgoing JWT secret.
func (a *AnchorPlatformAPIService) GetJWTToken(transactions []Transaction) (string, error) {
	if a.jwtManager == nil {
		return "", ErrJWTManagerNotSet
	}

	var txIDs []string
	for _, tx := range transactions {
		txIDs = append(txIDs, tx.TransactionValues.ID)
	}

	token, err := a.jwtManager.GenerateDefaultToken(strings.Join(txIDs, ","))
	if err != nil {
		return "", fmt.Errorf("error generating jwt token: %w", err)
	}

	return token, nil
}

// Ensuring that AnchorPlatformAPIService is implementing AnchorPlatformAPIServiceInterface.
var _ AnchorPlatformAPIServiceInterface = (*AnchorPlatformAPIService)(nil)
