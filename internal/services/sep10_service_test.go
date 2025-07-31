package services

import (
	"context"
	"testing"
	"time"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
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
		errorContains          string
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
			errorContains:          "parsing sep10 signing key",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service, err := NewSEP10Service(
				tc.jwtManager,
				tc.networkPassphrase,
				tc.sep10SigningPrivateKey,
				tc.baseURL,
				tc.models,
			)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
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
	t.Parallel()

	kp := keypair.MustRandom()

	realService, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://ultramar.imperium.com",
		&data.Models{},
	)
	require.NoError(t, err)

	mockService := NewMockSEP10Service(t)

	testCases := []struct {
		name          string
		service       SEP10Service
		req           ChallengeRequest
		expectError   bool
		errorContains string
		checkResponse func(t *testing.T, resp *ChallengeResponse)
	}{
		{
			name:    "✅ valid challenge request",
			service: realService,
			req: ChallengeRequest{
				Account:    kp.Address(),
				HomeDomain: "ultramar.imperium.com",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				assert.NotEmpty(t, resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)
			},
		},
		{
			name:    "✅ valid challenge with memo",
			service: realService,
			req: ChallengeRequest{
				Account:    kp.Address(),
				HomeDomain: "ultramar.imperium.com",
				Memo:       "12345",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				assert.NotEmpty(t, resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)
			},
		},
		{
			name:    "❌ invalid account",
			service: realService,
			req: ChallengeRequest{
				Account:    "invalid-account",
				HomeDomain: "ultramar.imperium.com",
			},
			expectError:   true,
			errorContains: "invalid account not a valid ed25519 public key",
		},
		{
			name:    "❌ invalid memo",
			service: realService,
			req: ChallengeRequest{
				Account:    kp.Address(),
				HomeDomain: "ultramar.imperium.com",
				Memo:       "invalid-memo",
			},
			expectError:   true,
			errorContains: "invalid memo must be a positive integer",
		},
		{
			name:    "❌ invalid home domain",
			service: realService,
			req: ChallengeRequest{
				Account:    kp.Address(),
				HomeDomain: "chaos.galaxy.com",
			},
			expectError:   true,
			errorContains: "invalid home_domain must match ultramar.imperium.com",
		},
		{
			name:    "✅ mock service success",
			service: mockService,
			req: ChallengeRequest{
				Account:    kp.Address(),
				HomeDomain: "ultramar.imperium.com",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ChallengeResponse) {
				assert.Equal(t, "mock_transaction", resp.Transaction)
				assert.Equal(t, "mock_network", resp.NetworkPassphrase)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if mockSvc, ok := tc.service.(*MockSEP10Service); ok {
				mockSvc.On("CreateChallenge", mock.Anything, tc.req).Return(
					&ChallengeResponse{
						Transaction:       "mock_transaction",
						NetworkPassphrase: "mock_network",
					}, nil,
				).Once()
			}

			resp, err := tc.service.CreateChallenge(context.Background(), tc.req)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
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

	realService, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://ultramar.imperium.com",
		&data.Models{},
	)
	require.NoError(t, err)

	mockService := NewMockSEP10Service(t)

	testCases := []struct {
		name          string
		service       SEP10Service
		req           ValidationRequest
		expectError   bool
		errorContains string
		checkResponse func(t *testing.T, resp *ValidationResponse)
	}{
		{
			name:    "❌ empty transaction",
			service: realService,
			req: ValidationRequest{
				Transaction: "",
			},
			expectError:   true,
			errorContains: "transaction is required",
		},
		{
			name:    "❌ invalid transaction",
			service: realService,
			req: ValidationRequest{
				Transaction: "invalid_transaction",
			},
			expectError:   true,
			errorContains: "reading challenge transaction",
		},
		{
			name:    "✅ mock service success",
			service: mockService,
			req: ValidationRequest{
				Transaction: "mock_transaction",
			},
			expectError: false,
			checkResponse: func(t *testing.T, resp *ValidationResponse) {
				assert.Equal(t, "mock_jwt_token", resp.Token)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if mockSvc, ok := tc.service.(*MockSEP10Service); ok {
				mockSvc.On("ValidateChallenge", mock.Anything, tc.req).Return(
					&ValidationResponse{
						Token: "mock_jwt_token",
					}, nil,
				).Once()
			}

			resp, err := tc.service.ValidateChallenge(context.Background(), tc.req)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
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

func TestSEP10Service_Integration_WithRealDB(t *testing.T) {
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	kp := keypair.MustRandom()

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://cadia.imperium.com",
		models,
	)
	require.NoError(t, err)

	t.Run("✅ integration test with real DB", func(t *testing.T) {
		t.Parallel()

		req := ChallengeRequest{
			Account:    kp.Address(),
			HomeDomain: "cadia.imperium.com",
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

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://fenris.imperium.com",
		&data.Models{},
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ getBaseDomain", func(t *testing.T) {
		t.Parallel()
		domain := sep10Service.getBaseDomain()
		assert.Equal(t, "fenris.imperium.com", domain)
	})

	t.Run("✅ isValidHomeDomain", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			domain      string
			expectValid bool
		}{
			{"fenris.imperium.com", true},
			{"chaos.galaxy.com", false},
			{"", false},
		}

		for _, tc := range testCases {
			t.Run(tc.domain, func(t *testing.T) {
				t.Parallel()
				isValid := sep10Service.isValidHomeDomain(tc.domain)
				assert.Equal(t, tc.expectValid, isValid)
			})
		}
	})

	t.Run("✅ getAllowedHomeDomains", func(t *testing.T) {
		t.Parallel()
		domains := sep10Service.getAllowedHomeDomains()
		assert.Len(t, domains, 1)
		assert.Equal(t, "fenris.imperium.com", domains[0])
	})

	t.Run("✅ getWebAuthDomain", func(t *testing.T) {
		t.Parallel()
		domain := sep10Service.getWebAuthDomain(context.Background())
		assert.Equal(t, "fenris.imperium.com", domain)
	})
}

func TestSEP10Service_IsValidHomeDomain_Comprehensive(t *testing.T) {
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
			t.Parallel()

			testService, err := NewSEP10Service(
				&anchorplatform.JWTManager{},
				"Test SDF Network ; September 2015",
				kp.Seed(),
				tc.baseURL,
				&data.Models{},
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

func TestSEP10Service_BuildChallengeTx(t *testing.T) {
	t.Parallel()

	kp := keypair.MustRandom()

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://maccrage.imperium.com",
		&data.Models{},
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	testCases := []struct {
		name            string
		clientAccountID string
		webAuthDomain   string
		homeDomain      string
		clientDomain    string
		memo            *txnbuild.MemoID
		expectError     bool
		errorContains   string
	}{
		{
			name:            "✅ valid challenge tx without memo",
			clientAccountID: kp.Address(),
			webAuthDomain:   "maccrage.imperium.com",
			homeDomain:      "maccrage.imperium.com",
			clientDomain:    "",
			memo:            nil,
			expectError:     false,
		},
		{
			name:            "✅ valid challenge tx with memo",
			clientAccountID: kp.Address(),
			webAuthDomain:   "maccrage.imperium.com",
			homeDomain:      "maccrage.imperium.com",
			clientDomain:    "",
			memo:            &[]txnbuild.MemoID{12345}[0],
			expectError:     false,
		},
		{
			name:            "✅ valid challenge tx with client domain",
			clientAccountID: kp.Address(),
			webAuthDomain:   "maccrage.imperium.com",
			homeDomain:      "maccrage.imperium.com",
			clientDomain:    "valhalla.imperium.com",
			memo:            nil,
			expectError:     false,
		},
		{
			name:            "❌ invalid account",
			clientAccountID: "invalid-account",
			webAuthDomain:   "maccrage.imperium.com",
			homeDomain:      "maccrage.imperium.com",
			clientDomain:    "",
			memo:            nil,
			expectError:     true,
			errorContains:   "is not a valid account id or muxed account",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tx, err := sep10Service.buildChallengeTx(
				tc.clientAccountID,
				tc.webAuthDomain,
				tc.homeDomain,
				tc.clientDomain,
				tc.memo,
			)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
				assert.Nil(t, tx)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, tx)

				expectedOps := 2
				if tc.clientDomain != "" {
					expectedOps = 3
				}
				assert.Len(t, tx.Operations(), expectedOps)

				if tc.clientDomain != "" {
					foundClientDomain := false
					for _, op := range tx.Operations() {
						if md, ok := op.(*txnbuild.ManageData); ok && md.Name == "client_domain" {
							assert.Equal(t, tc.clientDomain, string(md.Value))
							foundClientDomain = true
							break
						}
					}
					assert.True(t, foundClientDomain, "Client domain operation not found")
				}
			}
		})
	}
}

func TestSEP10Service_ExtractClientDomain(t *testing.T) {
	t.Parallel()

	kp := keypair.MustRandom()

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://baal.imperium.com",
		&data.Models{},
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ extract client domain from transaction", func(t *testing.T) {
		t.Parallel()

		tx, err := sep10Service.buildChallengeTx(
			kp.Address(),
			"baal.imperium.com",
			"baal.imperium.com",
			"caliban.imperium.com",
			nil,
		)
		require.NoError(t, err)

		clientDomain := sep10Service.extractClientDomain(tx)
		assert.Equal(t, "caliban.imperium.com", clientDomain)
	})

	t.Run("✅ no client domain in transaction", func(t *testing.T) {
		t.Parallel()

		tx, err := sep10Service.buildChallengeTx(
			kp.Address(),
			"baal.imperium.com",
			"baal.imperium.com",
			"",
			nil,
		)
		require.NoError(t, err)

		clientDomain := sep10Service.extractClientDomain(tx)
		assert.Equal(t, "", clientDomain)
	})
}

func TestSEP10Service_ValidateChallenge_WithRealDB(t *testing.T) {
	dbPool := testutils.GetDBConnectionPool(t)
	models, err := data.NewModels(dbPool)
	require.NoError(t, err)

	kp := keypair.MustRandom()

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://cadia.imperium.com",
		models,
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ validateClientDomain with real DB", func(t *testing.T) {
		t.Parallel()

		err := sep10Service.validateClientDomain(context.Background(), "nonexistent.imperium.com")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client domain")
	})
}

func TestSEP10Service_VerifySignatures(t *testing.T) {
	t.Parallel()

	kp := keypair.MustRandom()

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://baal.imperium.com",
		&data.Models{},
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ verifySignatures with valid transaction", func(t *testing.T) {
		t.Parallel()

		tx, err := sep10Service.buildChallengeTx(
			kp.Address(),
			"baal.imperium.com",
			"baal.imperium.com",
			"",
			nil,
		)
		require.NoError(t, err)

		clientKP := keypair.MustRandom()
		_, err = tx.Sign("Test SDF Network ; September 2015", clientKP)
		require.NoError(t, err)

		txBase64, err := tx.Base64()
		require.NoError(t, err)

		err = sep10Service.verifySignatures(
			txBase64,
			clientKP.Address(),
			"baal.imperium.com",
			[]string{"baal.imperium.com"},
			false,
		)
		require.Error(t, err)
	})
}

func TestSEP10Service_GenerateToken(t *testing.T) {
	t.Parallel()

	kp := keypair.MustRandom()

	jwtManager, err := anchorplatform.NewJWTManager("test-secret-key-123", 3600000)
	require.NoError(t, err)

	service, err := NewSEP10Service(
		jwtManager,
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://caliban.imperium.com",
		&data.Models{},
	)
	require.NoError(t, err)

	sep10Service := service.(*sep10Service)

	t.Run("✅ generateToken with valid transaction", func(t *testing.T) {
		t.Parallel()

		tx, err := sep10Service.buildChallengeTx(
			kp.Address(),
			"caliban.imperium.com",
			"caliban.imperium.com",
			"",
			nil,
		)
		require.NoError(t, err)

		resp, err := sep10Service.generateToken(
			tx,
			kp.Address(),
			"",
			"caliban.imperium.com",
			nil,
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

	service, err := NewSEP10Service(
		&anchorplatform.JWTManager{},
		"Test SDF Network ; September 2015",
		kp.Seed(),
		"https://fenris.imperium.com",
		models,
	)
	require.NoError(t, err)

	t.Run("✅ complete validation flow", func(t *testing.T) {
		t.Parallel()

		req := ChallengeRequest{
			Account:    kp.Address(),
			HomeDomain: "fenris.imperium.com",
		}

		challengeResp, err := service.CreateChallenge(context.Background(), req)
		require.NoError(t, err)
		assert.NotEmpty(t, challengeResp.Transaction)

		validationReq := ValidationRequest{
			Transaction: challengeResp.Transaction,
		}

		validationResp, err := service.ValidateChallenge(context.Background(), validationReq)
		require.Error(t, err)
		assert.Nil(t, validationResp)
	})
}

func TestChallengeRequest_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		req           ChallengeRequest
		expectError   bool
		errorContains string
	}{
		{
			name: "✅ valid request",
			req: ChallengeRequest{
				Account:    keypair.MustRandom().Address(),
				HomeDomain: "fenris.imperium.com",
			},
			expectError: false,
		},
		{
			name: "✅ valid request with memo",
			req: ChallengeRequest{
				Account:    keypair.MustRandom().Address(),
				HomeDomain: "fenris.imperium.com",
				Memo:       "12345",
			},
			expectError: false,
		},
		{
			name: "❌ empty account",
			req: ChallengeRequest{
				Account:    "",
				HomeDomain: "fenris.imperium.com",
			},
			expectError:   true,
			errorContains: "account is required",
		},
		{
			name: "❌ invalid account",
			req: ChallengeRequest{
				Account:    "invalid-account",
				HomeDomain: "fenris.imperium.com",
			},
			expectError:   true,
			errorContains: "invalid account not a valid ed25519 public key",
		},
		{
			name: "❌ invalid memo",
			req: ChallengeRequest{
				Account:    keypair.MustRandom().Address(),
				HomeDomain: "fenris.imperium.com",
				Memo:       "invalid-memo",
			},
			expectError:   true,
			errorContains: "invalid memo must be a positive integer",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.req.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidationRequest_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		req           ValidationRequest
		expectError   bool
		errorContains string
	}{
		{
			name: "✅ valid request",
			req: ValidationRequest{
				Transaction: "valid_transaction",
			},
			expectError: false,
		},
		{
			name: "❌ empty transaction",
			req: ValidationRequest{
				Transaction: "",
			},
			expectError:   true,
			errorContains: "transaction is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.req.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
