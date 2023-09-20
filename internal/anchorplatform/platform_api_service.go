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
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

var (
	ErrJWTManagerNotSet    = fmt.Errorf("jwt manager not set")
	ErrAuthNotEnforcedOnAP = fmt.Errorf("anchor platform is not enforcing authentication")
	ErrServiceUnavailable  = fmt.Errorf("anchor platform service is unavailable")
)

type APTransactionStatus string

const (
	APTransactionStatusCompleted     APTransactionStatus = "completed"
	APTransactionStatusError         APTransactionStatus = "error"
	APTransactionStatusPendingAnchor APTransactionStatus = "pending_anchor"
)

type AnchorPlatformAPIServiceInterface interface {
	PatchAnchorTransactionsPostRegistration(ctx context.Context, apTxPatch ...APSep24TransactionPatchPostRegistration) error
	IsAnchorProtectedByAuth(ctx context.Context) (bool, error)
}

type AnchorPlatformAPIService struct {
	HttpClient                    httpclient.HttpClientInterface
	AnchorPlatformBasePlatformURL string
	jwtManager                    *JWTManager
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

func (a *AnchorPlatformAPIService) PatchAnchorTransactionsPostRegistration(ctx context.Context, apTxPostRegistrationPatch ...APSep24TransactionPatchPostRegistration) error {
	var apTxPatches []APSep24TransactionPatch
	for _, patch := range apTxPostRegistrationPatch {
		apTxPatch, err := utils.ConvertType[APSep24TransactionPatchPostRegistration, APSep24TransactionPatch](patch)
		if err != nil {
			return fmt.Errorf("converting apTxPostRegistrationPatch into apTxPatch: %w", err)
		}
		apTxPatches = append(apTxPatches, APSep24TransactionPatch(apTxPatch))
	}

	return a.updateAnchorTransactions(ctx, apTxPatches...)
}

// updateAnchorTransactions will update the transactions on the anchor platform, according with the API documentation in
// https://developers.stellar.org/api/anchor-platform/resources/patch-transactions.
func (a *AnchorPlatformAPIService) updateAnchorTransactions(ctx context.Context, apTxPatch ...APSep24TransactionPatch) error {
	records := NewAPSep24TransactionRecordsFromPatches(apTxPatch...)

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

	token, err := a.GetJWTToken(apTxPatch...)
	if err != nil {
		return fmt.Errorf("getting jwt token in updateAnchorTransactions: %w", err)
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
		token, err = a.GetJWTToken()
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
func (a *AnchorPlatformAPIService) GetJWTToken(apTx ...APSep24TransactionPatch) (string, error) {
	if a.jwtManager == nil {
		return "", ErrJWTManagerNotSet
	}

	var txIDs []string
	for _, tx := range apTx {
		txIDs = append(txIDs, tx.ID)
	}

	token, err := a.jwtManager.GenerateDefaultToken(strings.Join(txIDs, ","))
	if err != nil {
		return "", fmt.Errorf("error generating jwt token: %w", err)
	}

	return token, nil
}

// Ensuring that AnchorPlatformAPIService is implementing AnchorPlatformAPIServiceInterface.
var _ AnchorPlatformAPIServiceInterface = (*AnchorPlatformAPIService)(nil)
