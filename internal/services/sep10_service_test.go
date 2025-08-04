package services

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/txnbuild"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/testutils"
)

func TestNewSEP10Service(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()

	testCases := []struct {
		name                   string
		jwtManager             *anchorplatform.JWTManager
		networkPassphrase      string
		sep10SigningPrivateKey string
		baseURL                string
		models                 *data.Models
		expectError            bool
		errMsg                 string
	}{
		{
			name:                   "✅ valid configuration",
			jwtManager:             &anchorplatform.JWTManager{},
			networkPassphrase:      "Test SDF Network ; September 2015",
			sep10SigningPrivateKey: kp.Seed(),
			baseURL:                "https://ultramar.imperium.com",
			models:                 &data.Models{},
			expectError:            false,
		},
		{
			name:                   "❌ invalid signing key",
			jwtManager:             &anchorplatform.JWTManager{},
			networkPassphrase:      "Test SDF Network ; September 2015",
			sep10SigningPrivateKey: "invalid-key",
			baseURL:                "https://ultramar.imperium.com",
			models:                 &data.Models{},
			expectError:            true,
			errMsg:                 "parsing sep10 signing key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockHorizonClient := &horizonclient.MockClient{}
			mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
				AccountID: "GDXC7OOSMJR3ZWNPYKHPHCSTP2SC6D4HY4SNKQMM7GEV6WRAMVCYA7XN",
				Thresholds: horizon.AccountThresholds{
					LowThreshold:  1,
					MedThreshold:  2,
					HighThreshold: 3,
				},
			}, nil)

			service, err := NewSEP10Service(
				tc.jwtManager,
				tc.networkPassphrase,
				tc.sep10SigningPrivateKey,
				tc.baseURL,
				tc.models,
				true,
				mockHorizonClient,
			)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				assert.Nil(t, service)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, service)

				sep10Service, ok := service.(*sep10Service)
				require.True(t, ok)
				assert.Equal(t, tc.jwtManager, sep10Service.JWTManager)
				assert.Equal(t, tc.networkPassphrase, sep10Service.NetworkPassphrase)
				assert.Equal(t, tc.baseURL, sep10Service.BaseURL)
				assert.Equal(t, tc.models, sep10Service.Models)
				assert.Equal(t, time.Hour*24, sep10Service.JWTExpiration)
				assert.Equal(t, time.Minute*15, sep10Service.AuthTimeout)
			}
		})
	}
}

func TestSEP10Service_CreateChallenge(t *testing.T) {
	kp := keypair.MustRandom()

	testCases := []struct {
		name          string
		req           ChallengeRequest
		expectError   bool
		errMsg        string
		checkResponse func(t *testing.T, resp *ChallengeResponse)
	}{
		{
			name: "✅ valid challenge request",
			req: ChallengeRequest{
				Account:      kp.Address(),
				HomeDomain:   "ultramar.imperium.com",
				ClientDomain: "test-client-domain.com",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				assert.NotEmpty(t, resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)
			},
		},
		{
			name: "✅ valid challenge with memo",
			req: ChallengeRequest{
				Account:      kp.Address(),
				HomeDomain:   "ultramar.imperium.com",
				ClientDomain: "test-client-domain.com",
				Memo:         "12345",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				assert.NotEmpty(t, resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)
			},
		},
		{
			name: "❌ invalid account",
			req: ChallengeRequest{
				Account:      "invalid-account",
				HomeDomain:   "ultramar.imperium.com",
				ClientDomain: "test-client-domain.com",
			},
			expectError: true,
			errMsg:      "invalid-account is not a valid account id",
		},
		{
			name: "❌ invalid home domain",
			req: ChallengeRequest{
				Account:      kp.Address(),
				HomeDomain:   "chaos.galaxy.com",
				ClientDomain: "test-client-domain.com",
			},
			expectError: true,
			errMsg:      "invalid home_domain must match ultramar.imperium.com",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockHorizonClient := &horizonclient.MockClient{}
			mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
				AccountID: kp.Address(),
				Thresholds: horizon.AccountThresholds{
					LowThreshold:  1,
					MedThreshold:  2,
					HighThreshold: 3,
				},
			}, nil)

			service, err := NewSEP10Service(
				&anchorplatform.JWTManager{},
				"Test SDF Network ; September 2015",
				kp.Seed(),
				"https://ultramar.imperium.com",
				&data.Models{},
				true,
				mockHorizonClient,
			)
			require.NoError(t, err)

			if !tc.expectError {
				sep10Service := service.(*sep10Service)
				mockHTTPClient := mocks.NewHttpClientMock(t)
				mockHTTPClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("SIGNING_KEY = \"" + kp.Address() + "\"\n")),
				}, nil)
				sep10Service.HTTPClient = mockHTTPClient
			}

			resp, err := service.CreateChallenge(context.Background(), tc.req)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)
				if tc.checkResponse != nil {
					tc.checkResponse(t, resp)
				}
			}
		})
	}
}

func TestSEP10Service_ValidateChallenge(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()

	mockHorizonClient := &horizonclient.MockClient{}
	mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
		AccountID: kp.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  1,
			MedThreshold:  2,
			HighThreshold: 3,
		},
	}, nil)

	testKP := keypair.MustRandom()

	sep10Service, err := NewSEP10Service(
		nil,
		"Test SDF Network ; September 2015",
		testKP.Seed(),
		"https://stellar.local:8000",
		nil,
		false,
		mockHorizonClient,
	)
	require.NoError(t, err)

	t.Run("❌ invalid transaction", func(t *testing.T) {
		req := ValidationRequest{
			Transaction: "invalid-transaction",
		}

		_, err := sep10Service.ValidateChallenge(context.Background(), req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "could not parse challenge")
	})
}

func TestSEP10Service_Integration_WithRealDB(t *testing.T) {
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	kp := keypair.MustRandom()

	mockHTTPClient := mocks.NewHttpClientMock(t)
	mockHTTPClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("SIGNING_KEY = \"" + kp.Address() + "\"\n")),
	}, nil)

	mockHorizonClient := &horizonclient.MockClient{}
	mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
		AccountID: kp.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  1,
			MedThreshold:  2,
			HighThreshold: 3,
		},
	}, nil)

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://cadia.imperium.com",
		models,
		true,
		mockHorizonClient,
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)
	sep10Service.HTTPClient = mockHTTPClient

	t.Run("✅ integration test with real DB", func(t *testing.T) {
		req := ChallengeRequest{
			Account:      kp.Address(),
			HomeDomain:   "cadia.imperium.com",
			ClientDomain: "test-client-domain.com",
		}

		resp, err := service.CreateChallenge(context.Background(), req)
		require.NoError(t, err)
		assert.NotEmpty(t, resp.Transaction)
		assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)
	})
}

func TestSEP10Service_HelperMethods(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()

	mockHorizonClient := &horizonclient.MockClient{}
	mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
		AccountID: kp.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  1,
			MedThreshold:  2,
			HighThreshold: 3,
		},
	}, nil)

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://stellar.local:8000",
		&data.Models{},
		true,
		mockHorizonClient,
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ getBaseDomain", func(t *testing.T) {
		baseDomain := sep10Service.getBaseDomain()
		assert.Equal(t, "stellar.local:8000", baseDomain)
	})

	t.Run("✅ getWebAuthDomain", func(t *testing.T) {
		webAuthDomain := sep10Service.getWebAuthDomain(context.Background())
		assert.Equal(t, "stellar.local:8000", webAuthDomain)
	})

	t.Run("✅ getAllowedHomeDomains", func(t *testing.T) {
		allowedDomains := sep10Service.getAllowedHomeDomains(context.Background())
		assert.Len(t, allowedDomains, 1)
		assert.Contains(t, allowedDomains, "stellar.local:8000")
	})

	t.Run("✅ isValidHomeDomain", func(t *testing.T) {
		assert.True(t, sep10Service.isValidHomeDomain("stellar.local:8000"))
		assert.True(t, sep10Service.isValidHomeDomain("subdomain.stellar.local:8000"))
		assert.False(t, sep10Service.isValidHomeDomain("other.domain"))
		assert.False(t, sep10Service.isValidHomeDomain(""))
	})
}

func TestSEP10Service_BuildChallengeTx(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()

	mockHorizonClient := &horizonclient.MockClient{}
	mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
		AccountID: kp.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  1,
			MedThreshold:  2,
			HighThreshold: 3,
		},
	}, nil)

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://stellar.local:8000",
		&data.Models{},
		true,
		mockHorizonClient,
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ valid challenge tx", func(t *testing.T) {
		tx, err := sep10Service.buildChallengeTx("GDXC7OOSMJR3ZWNPYKHPHCSTP2SC6D4HY4SNKQMM7GEV6WRAMVCYA7XN", "stellar.local:8000", "stellar.local:8000", "test-client-domain", "GDXC7OOSMJR3ZWNPYKHPHCSTP2SC6D4HY4SNKQMM7GEV6WRAMVCYA7XN", nil)
		assert.NoError(t, err)
		assert.NotNil(t, tx)

		operations := tx.Operations()
		assert.Len(t, operations, 3)

		// Check auth operation
		authOp, ok := operations[0].(*txnbuild.ManageData)
		assert.True(t, ok)
		assert.Equal(t, "stellar.local:8000 auth", authOp.Name)
		assert.Equal(t, "GDXC7OOSMJR3ZWNPYKHPHCSTP2SC6D4HY4SNKQMM7GEV6WRAMVCYA7XN", authOp.SourceAccount)

		// Check web_auth_domain operation
		webAuthOp, ok := operations[1].(*txnbuild.ManageData)
		assert.True(t, ok)
		assert.Equal(t, "web_auth_domain", webAuthOp.Name)
		assert.Equal(t, "stellar.local:8000", string(webAuthOp.Value))
		assert.Equal(t, sep10Service.Sep10SigningKeypair.Address(), webAuthOp.SourceAccount)

		// Check client_domain operation
		clientDomainOp, ok := operations[2].(*txnbuild.ManageData)
		assert.True(t, ok)
		assert.Equal(t, "client_domain", clientDomainOp.Name)
		assert.Equal(t, "test-client-domain", string(clientDomainOp.Value))
		assert.Equal(t, "GDXC7OOSMJR3ZWNPYKHPHCSTP2SC6D4HY4SNKQMM7GEV6WRAMVCYA7XN", clientDomainOp.SourceAccount)
	})

	t.Run("❌ invalid account", func(t *testing.T) {
		_, err := sep10Service.buildChallengeTx("invalid-account", "stellar.local:8000", "stellar.local:8000", "test-client-domain", "GDXC7OOSMJR3ZWNPYKHPHCSTP2SC6D4HY4SNKQMM7GEV6WRAMVCYA7XN", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "is not a valid account id")
	})

	t.Run("❌ timeout too short", func(t *testing.T) {
		sep10Service.AuthTimeout = time.Millisecond
		_, err := sep10Service.buildChallengeTx("GDXC7OOSMJR3ZWNPYKHPHCSTP2SC6D4HY4SNKQMM7GEV6WRAMVCYA7XN", "stellar.local:8000", "stellar.local:8000", "test-client-domain", "GDXC7OOSMJR3ZWNPYKHPHCSTP2SC6D4HY4SNKQMM7GEV6WRAMVCYA7XN", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provided timebound must be at least 1s")
	})
}

func TestSEP10Service_IsValidHomeDomain(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()

	testCases := []struct {
		name        string
		baseURL     string
		homeDomain  string
		expectValid bool
	}{
		// Exact matches
		{"exact match", "https://fenris.imperium.com", "fenris.imperium.com", true},
		{"exact match with https", "https://fenris.imperium.com", "fenris.imperium.com", true},
		{"exact match with http", "http://fenris.imperium.com", "fenris.imperium.com", true},

		// Subdomain matches
		{"subdomain match", "https://fenris.imperium.com", "api.fenris.imperium.com", true},
		{"subdomain match", "https://fenris.imperium.com", "auth.fenris.imperium.com", true},
		{"nested subdomain", "https://fenris.imperium.com", "api.v1.fenris.imperium.com", true},

		// Trailing slash edge cases
		{"base URL with trailing slash", "https://fenris.imperium.com/", "fenris.imperium.com", true},
		{"home domain with trailing slash", "https://fenris.imperium.com", "fenris.imperium.com/", false},
		{"both with trailing slash", "https://fenris.imperium.com/", "fenris.imperium.com/", false},
		{"subdomain with trailing slash", "https://fenris.imperium.com", "api.fenris.imperium.com/", false},

		// Invalid cases
		{"different domain", "https://fenris.imperium.com", "chaos.galaxy.com", false},
		{"empty home domain", "https://fenris.imperium.com", "", false},
		{"empty base URL", "", "fenris.imperium.com", false},
		{"partial match", "https://fenris.imperium.com", "fenris.imperium.com.evil.com", false},
		{"suffix but not subdomain", "https://fenris.imperium.com", "myfenris.imperium.com", false},

		// Case sensitivity
		{"case insensitive base", "https://FENRIS.IMPERIUM.COM", "fenris.imperium.com", true},
		{"case insensitive home domain", "https://fenris.imperium.com", "FENRIS.IMPERIUM.COM", true},
		{"mixed case subdomain", "https://fenris.imperium.com", "API.fenris.imperium.com", true},

		// Port numbers
		{"base URL with port", "https://fenris.imperium.com:8080", "fenris.imperium.com:8080", true},
		{"home domain with port", "https://fenris.imperium.com", "fenris.imperium.com:8080", false},

		// Path in base URL
		{"base URL with path", "https://fenris.imperium.com/api", "fenris.imperium.com", true},
		{"base URL with complex path", "https://fenris.imperium.com/api/v1/auth", "fenris.imperium.com", true},

		// Query parameters
		{"base URL with query", "https://fenris.imperium.com?param=value", "fenris.imperium.com", true},
		{"base URL with fragment", "https://fenris.imperium.com#section", "fenris.imperium.com", true},

		// Edge cases with dots
		{"domain with dots", "https://fenris.imperium.com", "fenris.imperium.com", true},
		{"subdomain with dots", "https://fenris.imperium.com", "api.fenris.imperium.com", true},
		{"fake subdomain", "https://fenris.imperium.com", "fenris.imperium.com.evil.com", false},

		// Special characters
		{"base URL with special chars", "https://fenris-imperium.com", "fenris-imperium.com", true},
		{"home domain with special chars", "https://fenris.imperium.com", "fenris-imperium.com", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockHorizonClient := &horizonclient.MockClient{}
			mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
				AccountID: kp.Address(),
				Thresholds: horizon.AccountThresholds{
					LowThreshold:  1,
					MedThreshold:  2,
					HighThreshold: 3,
				},
			}, nil)

			testService, err := NewSEP10Service(
				&anchorplatform.JWTManager{},
				"Test SDF Network ; September 2015",
				kp.Seed(),
				tc.baseURL,
				&data.Models{},
				true,
				mockHorizonClient,
			)
			require.NoError(t, err)

			testSep10Service := testService.(*sep10Service)
			isValid := testSep10Service.isValidHomeDomain(tc.homeDomain)
			assert.Equal(t, tc.expectValid, isValid,
				"BaseURL: %s, HomeDomain: %s, Expected: %v, Got: %v",
				tc.baseURL, tc.homeDomain, tc.expectValid, isValid)
		})
	}
}

func TestSEP10Service_VerifySignatures(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()

	mockHorizonClient := &horizonclient.MockClient{}
	mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
		AccountID: kp.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  1,
			MedThreshold:  2,
			HighThreshold: 3,
		},
	}, nil)

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://baal.imperium.com",
		&data.Models{},
		true,
		mockHorizonClient,
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ verifySignatures with valid transaction", func(t *testing.T) {
		tx, err := sep10Service.buildChallengeTx(
			kp.Address(),
			"baal.imperium.com",
			"baal.imperium.com",
			"test-client-domain",
			kp.Address(),
			nil,
		)
		require.NoError(t, err)

		clientKP := keypair.MustRandom()
		_, err = tx.Sign("Test SDF Network ; September 2015", clientKP)
		require.NoError(t, err)

		txBase64, err := tx.Base64()
		require.NoError(t, err)

		mockAccount := &horizon.Account{
			AccountID: clientKP.Address(),
			Thresholds: horizon.AccountThresholds{
				LowThreshold:  1,
				MedThreshold:  2,
				HighThreshold: 3,
			},
		}

		err = sep10Service.verifySignaturesWithThreshold(
			txBase64,
			clientKP.Address(),
			"baal.imperium.com",
			[]string{"baal.imperium.com"},
			mockAccount,
		)
		require.Error(t, err)
	})
}

func TestSEP10Service_GenerateToken(t *testing.T) {
	t.Parallel()
	kp := keypair.MustRandom()

	jwtManager, err := anchorplatform.NewJWTManager("test-secret-key-123", 3600000)
	require.NoError(t, err)

	mockHorizonClient := &horizonclient.MockClient{}
	mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
		AccountID: kp.Address(),
		Thresholds: horizon.AccountThresholds{
			LowThreshold:  1,
			MedThreshold:  2,
			HighThreshold: 3,
		},
	}, nil)

	service, err := NewSEP10Service(
		jwtManager,
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://caliban.imperium.com",
		&data.Models{},
		true,
		mockHorizonClient,
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ generateToken with valid transaction", func(t *testing.T) {
		tx, err := sep10Service.buildChallengeTx(
			kp.Address(),
			"caliban.imperium.com",
			"caliban.imperium.com",
			"test-client-domain",
			kp.Address(),
			nil,
		)
		require.NoError(t, err)

		resp, err := sep10Service.generateToken(
			tx,
			kp.Address(),
			"caliban.imperium.com",
			nil,
			"test-client-domain",
		)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.NotEmpty(t, resp.Token)
	})
}

func TestSEP10Service_ValidateChallenge_CompleteFlow(t *testing.T) {
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	kp := keypair.MustRandom()

	mockHTTPClient := mocks.NewHttpClientMock(t)
	mockHTTPClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("SIGNING_KEY = \"" + kp.Address() + "\"\n")),
	}, nil)

	mockHorizonClient := &horizonclient.MockClient{}
	mockHorizonClient.On("AccountDetail", mock.AnythingOfType("horizonclient.AccountRequest")).Return(horizon.Account{
		AccountID:  kp.Address(),
		Thresholds: horizon.AccountThresholds{},
	}, nil)

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://fenris.imperium.com",
		models,
		true,
		mockHorizonClient,
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)
	sep10Service.HTTPClient = mockHTTPClient

	t.Run("✅ complete validation flow", func(t *testing.T) {
		req := ChallengeRequest{
			Account:      kp.Address(),
			HomeDomain:   "fenris.imperium.com",
			ClientDomain: "test-client-domain.com",
		}

		challengeResp, err := service.CreateChallenge(context.Background(), req)
		require.NoError(t, err)
		assert.NotEmpty(t, challengeResp.Transaction)

		validationReq := ValidationRequest{
			Transaction: challengeResp.Transaction,
		}

		validationResp, err := service.ValidateChallenge(context.Background(), validationReq)
		require.NoError(t, err)
		assert.NotNil(t, validationResp)
		assert.NotEmpty(t, validationResp.Token)
	})
}

func TestChallengeRequest_Validate(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		req         ChallengeRequest
		expectError bool
		errMsg      string
	}{
		{
			name: "✅ valid request",
			req: ChallengeRequest{
				Account:      keypair.MustRandom().Address(),
				HomeDomain:   "fenris.imperium.com",
				ClientDomain: "test-client-domain.com",
			},
			expectError: false,
		},
		{
			name: "✅ valid request with memo",
			req: ChallengeRequest{
				Account:      keypair.MustRandom().Address(),
				HomeDomain:   "fenris.imperium.com",
				ClientDomain: "test-client-domain.com",
				Memo:         "12345",
			},
			expectError: false,
		},
		{
			name: "❌ empty account",
			req: ChallengeRequest{
				Account:    "",
				HomeDomain: "fenris.imperium.com",
			},
			expectError: true,
			errMsg:      "account is required",
		},
		{
			name: "❌ invalid account",
			req: ChallengeRequest{
				Account:    "invalid-account",
				HomeDomain: "fenris.imperium.com",
			},
			expectError: true,
			errMsg:      "invalid account not a valid ed25519 public key",
		},
		{
			name: "❌ missing client_domain",
			req: ChallengeRequest{
				Account:    keypair.MustRandom().Address(),
				HomeDomain: "fenris.imperium.com",
			},
			expectError: true,
			errMsg:      "client_domain is required",
		},
		{
			name: "❌ invalid memo",
			req: ChallengeRequest{
				Account:      keypair.MustRandom().Address(),
				HomeDomain:   "fenris.imperium.com",
				ClientDomain: "test-client-domain.com",
				Memo:         "invalid-memo",
			},
			expectError: true,
			errMsg:      "invalid memo must be a positive integer",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
