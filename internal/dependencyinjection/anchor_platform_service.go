package dependencyinjection

import (
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

const anchorPlatformAPIServiceInstanceName = "anchor_platform_api_service_instance"

func NewAnchorPlatformAPIService(anchorPlatformBasePlatformURL, anchorPlatformOutgoingJWTSecret string) (anchorplatform.AnchorPlatformAPIServiceInterface, error) {
	// TODO: validate if anchorPlatformBasePlatformURL and anchorPlatformOutgoingJWTSecret are not empty
	if anchorPlatformBasePlatformURL == "" {
		return nil, fmt.Errorf("anchor platform base platform url cannot be empty")
	}

	if anchorPlatformOutgoingJWTSecret == "" {
		return nil, fmt.Errorf("anchor platform outgoing JWT secret cannot be empty")
	}

	log.Infof("⚙️ Setting default anchor platform APO Service")
	// Setup Anchor Platform API Service
	anchorPlatformAPIService, err := anchorplatform.NewAnchorPlatformAPIService(httpclient.DefaultClient(), anchorPlatformBasePlatformURL, anchorPlatformOutgoingJWTSecret)
	if err != nil {
		return nil, fmt.Errorf("creating Anchor Platform API service: %w", err)
	}

	setInstance(anchorPlatformAPIServiceInstanceName, anchorPlatformAPIService)
	return anchorPlatformAPIService, nil
}
