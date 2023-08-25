package dependencyinjection

import (
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient"
	"github.com/stretchr/testify/require"
)

func Test_NewAnchorPlatformAPIService(t *testing.T) {
	testingCases := []struct {
		name                            string
		anchorPlatformBasePlatformURL   string
		anchorPlatformOutgoingJWTSecret string
		wantErrContains                 string
	}{
		{
			name:                            "return an error if anchorPlatformBasePlatformURL is empty",
			anchorPlatformBasePlatformURL:   "",
			anchorPlatformOutgoingJWTSecret: "",
			wantErrContains:                 "anchor platform base platform url cannot be empty",
		},
		{
			name:                            "return an error if anchorPlatformOutgoingJWTSecret is empty",
			anchorPlatformBasePlatformURL:   "https://test.com",
			anchorPlatformOutgoingJWTSecret: "",
			wantErrContains:                 "anchor platform outgoing JWT secret cannot be empty",
		},
		{
			name:                            "🎉 successfully creates a new instance if none exist before",
			anchorPlatformBasePlatformURL:   "https://test.com",
			anchorPlatformOutgoingJWTSecret: "jwt_secret_1234567890",
			wantErrContains:                 "",
		},
	}

	for _, tc := range testingCases {
		t.Run(tc.name, func(t *testing.T) {
			defer ClearInstancesTestHelper(t)

			gotResult, err := NewAnchorPlatformAPIService(tc.anchorPlatformBasePlatformURL, tc.anchorPlatformOutgoingJWTSecret)
			if tc.wantErrContains != "" {
				require.ErrorContains(t, err, tc.wantErrContains)
				require.Nil(t, gotResult)
			} else {
				require.NoError(t, err)
				require.NotNil(t, gotResult)

				wantResult, err := anchorplatform.NewAnchorPlatformAPIService(httpclient.DefaultClient(), tc.anchorPlatformBasePlatformURL, tc.anchorPlatformOutgoingJWTSecret)
				require.NoError(t, err)
				require.Equal(t, wantResult, gotResult)
			}
		})
	}
}

func Test_NewAnchorPlatformAPIService_existingInstanceIsReturned(t *testing.T) {
	anchorPlatformBasePlatformURL := "https://test.com"
	anchorPlatformOutgoingJWTSecret := "jwt_secret_1234567890"

	defer ClearInstancesTestHelper(t)

	// STEP 1: assert that the instance is nil
	_, ok := dependenciesStoreMap[anchorPlatformAPIServiceInstanceName]
	require.False(t, ok)

	// STEP 2: create a new instance
	apService1, err := NewAnchorPlatformAPIService(anchorPlatformBasePlatformURL, anchorPlatformOutgoingJWTSecret)
	require.NoError(t, err)
	require.NotNil(t, apService1)

	// STEP 3: assert that the instance is not nil
	storedAPService, ok := dependenciesStoreMap[anchorPlatformAPIServiceInstanceName]
	require.True(t, ok)
	require.NotNil(t, storedAPService)
	require.True(t, apService1 == storedAPService)

	// STEP 4: create a new instance
	apService2, err := NewAnchorPlatformAPIService(anchorPlatformBasePlatformURL, anchorPlatformOutgoingJWTSecret)
	require.NoError(t, err)
	require.NotNil(t, apService2)

	// STEP 5: assert that the returned object is the same as the stored one
	require.Equal(t, apService1, apService2)
	require.True(t, apService1 == apService2)
}
