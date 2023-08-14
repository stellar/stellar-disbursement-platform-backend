package anchorplatform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

var ErrJWTManagerNotSet = fmt.Errorf("jwt manager not set")

// TODO update with the PlatformAPI endpoints
type AnchorPlatformAPIServiceInterface interface {
	UpdateAnchorTransactions(ctx context.Context, transactions []Transaction) error
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
	apService := AnchorPlatformAPIService{
		HttpClient:                    httpClient,
		AnchorPlatformBasePlatformURL: anchorPlatformBasePlatformURL,
	}

	if anchorPlatformOutgoingJWTSecret != "" {
		const expirationMiliseconds = 5000
		jwtManager, err := NewJWTManager(anchorPlatformOutgoingJWTSecret, expirationMiliseconds)
		if err != nil {
			return nil, fmt.Errorf("error creating jwt manager: %w", err)
		}
		apService.jwtManager = jwtManager
	}

	return &apService, nil
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

	// If the service is configured with an outgoing JWT secret, we'll generate a JWT token and add it to the request.
	token, err := a.GetJWTToken(transactions)
	if err != nil {
		if !errors.Is(err, ErrJWTManagerNotSet) {
			return fmt.Errorf("error getting jwt token in UpdateAnchorTransactions: %w", err)
		}
		log.Ctx(ctx).Warn("JWT secret not set, skipping JWT token generation")
	} else {
		request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	}

	response, err := a.HttpClient.Do(request)
	if err != nil {
		return fmt.Errorf("error making request to anchor platform: %w", err)
	}

	if response.StatusCode/100 != 2 {
		return fmt.Errorf("error updating transaction on anchor platform, response.StatusCode: %d", response.StatusCode)
	}

	return nil
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
