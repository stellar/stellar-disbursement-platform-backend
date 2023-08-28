package dependencyinjection

import (
	"fmt"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
)

const anchorPlatformAPIServiceInstanceName = "anchor_platform_api_service_instance"

func NewAnchorPlatformAPIService(anchorPlatformBasePlatformURL, anchorPlatformOutgoingJWTSecret string) (anchorplatform.AnchorPlatformAPIServiceInterface, error) {
	if anchorPlatformBasePlatformURL == "" {
		return nil, fmt.Errorf("anchor platform base platform url cannot be empty")
	}
	if anchorPlatformOutgoingJWTSecret == "" {
		return nil, fmt.Errorf("anchor platform outgoing JWT secret cannot be empty")
	}

	// If there is already an instance of the service, we return the same instance
	if instance, ok := dependenciesStoreMap[anchorPlatformAPIServiceInstanceName]; ok {
		if anchorPlatformAPIService, ok := instance.(anchorplatform.AnchorPlatformAPIServiceInterface); ok {
			return anchorPlatformAPIService, nil
		}
		return nil, fmt.Errorf("trying to cast pre-existing SMS client for depencency injection")
	}

	// Setup Anchor Platform API Service
	log.Infof("⚙️ Setting default anchor platform API Service")
	anchorPlatformAPIService, err := anchorplatform.NewAnchorPlatformAPIService(httpclient.DefaultClient(), anchorPlatformBasePlatformURL, anchorPlatformOutgoingJWTSecret)
	if err != nil {
		return nil, fmt.Errorf("creating Anchor Platform API service: %w", err)
	}
	setInstance(anchorPlatformAPIServiceInstanceName, anchorPlatformAPIService)

	return anchorPlatformAPIService, nil
}
