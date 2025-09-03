package integrationtests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	httpclientMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httpclient/mocks"
)

func createMockResponse(statusCode int, body any) *http.Response {
	var bodyBytes []byte
	if body != nil {
		if str, ok := body.(string); ok {
			bodyBytes = []byte(str)
		} else {
			jsonBytes, _ := json.Marshal(body)
			bodyBytes = jsonBytes
		}
	}

	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewBuffer(bodyBytes)),
		Header:     make(http.Header),
	}
}

func createTestKeypairs() (*keypair.Full, *keypair.Full) {
	receiverKP := keypair.MustRandom()
	clientDomainKP := keypair.MustRandom()

	return receiverKP, clientDomainKP
}

func createTestChallengeTransaction(t *testing.T, receiverAccount, clientDomainAccount string) *txnbuild.Transaction {
	serverKP := keypair.MustRandom()

	sa := txnbuild.SimpleAccount{
		AccountID: serverKP.Address(),
		Sequence:  -1,
	}

	operations := []txnbuild.Operation{
		&txnbuild.ManageData{
			SourceAccount: receiverAccount,
			Name:          "test.stellar.local:8000 auth",
			Value:         []byte("test-nonce-64-bytes-long-random-string-for-testing-purposes"),
		},
		&txnbuild.ManageData{
			SourceAccount: serverKP.Address(),
			Name:          "web_auth_domain",
			Value:         []byte("test.stellar.local:8000"),
		},
	}

	if clientDomainAccount != "" {
		operations = append(operations, &txnbuild.ManageData{
			SourceAccount: clientDomainAccount,
			Name:          "client_domain",
			Value:         []byte("test.stellar.local"),
		})
	}

	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sa,
		IncrementSequenceNum: true,
		Operations:           operations,
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewTimebounds(time.Now().UTC().Unix(), time.Now().UTC().Add(5*time.Minute).Unix()),
		},
	}

	tx, err := txnbuild.NewTransaction(txParams)
	require.NoError(t, err)

	return tx
}

func TestSDPSepServicesIntegrationTests_GetSEP10Challenge(t *testing.T) {
	t.Parallel()

	receiverKP, clientDomainKP:= createTestKeypairs()

	testCases := []struct {
		name           string
		setupMock      func(*httpclientMocks.HttpClientMock)
		expectedError  bool
		expectedStatus int
		checkResponse  func(*testing.T, *SEP10ChallengeResponse)
	}{
		{
			name: "✅ successful challenge creation",
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				challengeTx := createTestChallengeTransaction(t, receiverKP.Address(), clientDomainKP.Address())
				txBase64, _ := challengeTx.Base64()

				expectedResponse := SEP10ChallengeResponse{
					Transaction:       txBase64,
					NetworkPassphrase: "Test SDF Network ; September 2015",
				}

				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusOK, expectedResponse), nil,
				).Once()
			},
			expectedError:  false,
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, resp *SEP10ChallengeResponse) {
				assert.NotEmpty(t, resp.Transaction)
				assert.Equal(t, "Test SDF Network ; September 2015", resp.NetworkPassphrase)
				assert.NotNil(t, resp.ParsedTx)
			},
		},
		{
			name: "❌ server error",
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusInternalServerError, "Internal Server Error"), nil,
				).Once()
			},
			expectedError:  true,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "❌ invalid response format",
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusOK, "invalid json"), nil,
				).Once()
			},
			expectedError:  true,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &httpclientMocks.HttpClientMock{}
			tc.setupMock(mockClient)

			service := &SDPSepServicesIntegrationTests{
				HTTPClient:               mockClient,
				SDPBaseURL:               "https://test.stellar.local:8000",
				TenantName:               "test",
				ReceiverAccountPublicKey: receiverKP.Address(),
				ClientDomain:             "test.stellar.local", 
				ClientDomainPrivateKey:   "",                   
				HomeDomain:               "localhost:8000",     
			}

			ctx := context.Background()
			response, err := service.GetSEP10Challenge(ctx)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tc.checkResponse != nil {
					tc.checkResponse(t, response)
				}
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestSDPSepServicesIntegrationTests_SignSEP10Challenge(t *testing.T) {
	t.Parallel()

	receiverKP, clientDomainKP := createTestKeypairs()

	testCases := []struct {
		name          string
		challenge     *SEP10ChallengeResponse
		clientDomain  string
		expectedError bool
		checkResponse func(*testing.T, *SignedSEP10Challenge)
	}{
		{
			name: "✅ successful signing with receiver key only",
			challenge: &SEP10ChallengeResponse{
				Transaction:       "test-transaction",
				NetworkPassphrase: "Test SDF Network ; September 2015",
				ParsedTx:          createTestChallengeTransaction(t, receiverKP.Address(), ""),
			},
			clientDomain:  "",
			expectedError: false,
			checkResponse: func(t *testing.T, resp *SignedSEP10Challenge) {
				assert.NotEmpty(t, resp.SignedTransaction)
				assert.Equal(t, "test-transaction", resp.Transaction)
			},
		},
		{
			name: "✅ successful signing with client domain key",
			challenge: &SEP10ChallengeResponse{
				Transaction:       "test-transaction",
				NetworkPassphrase: "Test SDF Network ; September 2015",
				ParsedTx:          createTestChallengeTransaction(t, receiverKP.Address(), clientDomainKP.Address()),
			},
			clientDomain:  "test.stellar.local",
			expectedError: false,
			checkResponse: func(t *testing.T, resp *SignedSEP10Challenge) {
				assert.NotEmpty(t, resp.SignedTransaction)
			},
		},
		{
			name: "❌ malformed receiver private key",
			challenge: &SEP10ChallengeResponse{
				Transaction:       "test-transaction",
				NetworkPassphrase: "Test SDF Network ; September 2015",
				ParsedTx:          createTestChallengeTransaction(t, receiverKP.Address(), ""),
			},
			clientDomain:  "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var privateKey string
			if tc.name == "❌ malformed receiver private key" {
				privateKey = "invalid-private-key"
			} else {
				privateKey = receiverKP.Seed()
			}

			service := &SDPSepServicesIntegrationTests{
				ReceiverAccountPrivateKey: privateKey,
				ClientDomain:              tc.clientDomain,
			}

			response, err := service.SignSEP10Challenge(tc.challenge)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tc.checkResponse != nil {
					tc.checkResponse(t, response)
				}
			}
		})
	}
}

func TestSDPSepServicesIntegrationTests_ValidateSEP10Challenge(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		signedChallenge *SignedSEP10Challenge
		setupMock       func(*httpclientMocks.HttpClientMock)
		expectedError   bool
		expectedToken   string
	}{
		{
			name: "✅ successful challenge validation",
			signedChallenge: &SignedSEP10Challenge{
				SEP10ChallengeResponse: &SEP10ChallengeResponse{
					Transaction:       "test-transaction",
					NetworkPassphrase: "Test SDF Network ; September 2015",
				},
				SignedTransaction: "signed-transaction-base64",
			},
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				expectedResponse := SEP10AuthToken{
					Token: "jwt-token-here",
				}

				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusOK, expectedResponse), nil,
				).Once()
			},
			expectedError: false,
			expectedToken: "jwt-token-here",
		},
		{
			name: "❌ validation failed",
			signedChallenge: &SignedSEP10Challenge{
				SEP10ChallengeResponse: &SEP10ChallengeResponse{
					Transaction:       "test-transaction",
					NetworkPassphrase: "Test SDF Network ; September 2015",
				},
				SignedTransaction: "invalid-transaction",
			},
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusBadRequest, "Invalid transaction"), nil,
				).Once()
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &httpclientMocks.HttpClientMock{}
			tc.setupMock(mockClient)

			service := &SDPSepServicesIntegrationTests{
				HTTPClient: mockClient,
				SDPBaseURL: "https://test.stellar.local:8000",
			}

			ctx := context.Background()
			response, err := service.ValidateSEP10Challenge(ctx, tc.signedChallenge)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, tc.expectedToken, response.Token)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestSDPSepServicesIntegrationTests_InitiateSEP24Deposit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		sep10Token    *SEP10AuthToken
		setupMock     func(*httpclientMocks.HttpClientMock)
		expectedError bool
		checkResponse func(*testing.T, *SEP24DepositResponse)
	}{
		{
			name: "✅ successful deposit initiation",
			sep10Token: &SEP10AuthToken{
				Token: "valid-jwt-token",
			},
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				expectedResponse := SEP24DepositResponse{
					Type:          "interactive_customer_info_needed",
					URL:           "https://test.stellar.local:8000/sep24/interactive?token=sep24-token",
					TransactionID: "tx-123",
				}

				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusOK, expectedResponse), nil,
				).Once()
			},
			expectedError: false,
			checkResponse: func(t *testing.T, resp *SEP24DepositResponse) {
				assert.Equal(t, "tx-123", resp.TransactionID)
				assert.Equal(t, "sep24-token", resp.Token)
				assert.Contains(t, resp.URL, "token=sep24-token")
			},
		},
		{
			name: "❌ unauthorized",
			sep10Token: &SEP10AuthToken{
				Token: "invalid-token",
			},
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusUnauthorized, "Unauthorized"), nil,
				).Once()
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &httpclientMocks.HttpClientMock{}
			tc.setupMock(mockClient)

			service := &SDPSepServicesIntegrationTests{
				HTTPClient:         mockClient,
				SDPBaseURL:         "https://test.stellar.local:8000",
				DisbursedAssetCode: "USDC",
			}

			ctx := context.Background()
			response, err := service.InitiateSEP24Deposit(ctx, tc.sep10Token)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tc.checkResponse != nil {
					tc.checkResponse(t, response)
				}
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestSDPSepServicesIntegrationTests_GetSEP24Transaction(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		sep10Token    *SEP10AuthToken
		transactionID string
		setupMock     func(*httpclientMocks.HttpClientMock)
		expectedError bool
		checkResponse func(*testing.T, *SEP24TransactionStatus)
	}{
		{
			name: "✅ successful transaction status retrieval",
			sep10Token: &SEP10AuthToken{
				Token: "valid-jwt-token",
			},
			transactionID: "tx-123",
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				expectedResponse := SEP24TransactionStatus{
					Transaction: struct {
						ID              string     `json:"id"`
						Kind            string     `json:"kind"`
						Status          string     `json:"status"`
						MoreInfoURL     string     `json:"more_info_url,omitempty"`
						To              string     `json:"to,omitempty"`
						DepositMemo     string     `json:"deposit_memo,omitempty"`
						DepositMemoType string     `json:"deposit_memo_type,omitempty"`
						StartedAt       time.Time  `json:"started_at"`
						CompletedAt     *time.Time `json:"completed_at,omitempty"`
					}{
						ID:        "tx-123",
						Kind:      "deposit",
						Status:    "pending_user_transfer_start",
						StartedAt: time.Now().UTC(),
					},
				}

				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusOK, expectedResponse), nil,
				).Once()
			},
			expectedError: false,
			checkResponse: func(t *testing.T, resp *SEP24TransactionStatus) {
				assert.Equal(t, "tx-123", resp.Transaction.ID)
				assert.Equal(t, "deposit", resp.Transaction.Kind)
				assert.Equal(t, "pending_user_transfer_start", resp.Transaction.Status)
			},
		},
		{
			name: "❌ transaction not found",
			sep10Token: &SEP10AuthToken{
				Token: "valid-jwt-token",
			},
			transactionID: "nonexistent-tx",
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusNotFound, "Transaction not found"), nil,
				).Once()
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &httpclientMocks.HttpClientMock{}
			tc.setupMock(mockClient)

			service := &SDPSepServicesIntegrationTests{
				HTTPClient: mockClient,
				SDPBaseURL: "https://test.stellar.local:8000",
			}

			ctx := context.Background()
			response, err := service.GetSEP24Transaction(ctx, tc.sep10Token, tc.transactionID)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tc.checkResponse != nil {
					tc.checkResponse(t, response)
				}
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestSDPSepServicesIntegrationTests_CompleteReceiverRegistration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		sep24Token       string
		registrationData *ReceiverRegistrationRequest
		setupMock        func(*httpclientMocks.HttpClientMock)
		expectedError    bool
	}{
		{
			name:       "✅ successful registration completion",
			sep24Token: "valid-sep24-token",
			registrationData: &ReceiverRegistrationRequest{
				PhoneNumber:       "+1234567890",
				OTP:               "123456",
				VerificationValue: "+1234567890",
				VerificationField: "phone_number",
				ReCAPTCHAToken:    "recaptcha-token",
			},
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusOK, "Success"), nil,
				).Once()
			},
			expectedError: false,
		},
		{
			name:       "✅ successful registration with email",
			sep24Token: "valid-sep24-token",
			registrationData: &ReceiverRegistrationRequest{
				Email:             "test@example.com",
				OTP:               "123456",
				VerificationValue: "test@example.com",
				VerificationField: "email",
				ReCAPTCHAToken:    "recaptcha-token",
			},
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusCreated, "Success"), nil,
				).Once()
			},
			expectedError: false,
		},
		{
			name:       "❌ invalid token",
			sep24Token: "invalid-token",
			registrationData: &ReceiverRegistrationRequest{
				PhoneNumber:       "+1234567890",
				OTP:               "123456",
				VerificationValue: "+1234567890",
				VerificationField: "phone_number",
				ReCAPTCHAToken:    "recaptcha-token",
			},
			setupMock: func(mockClient *httpclientMocks.HttpClientMock) {
				mockClient.On("Do", mock.AnythingOfType("*http.Request")).Return(
					createMockResponse(http.StatusUnauthorized, "Invalid token"), nil,
				).Once()
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockClient := &httpclientMocks.HttpClientMock{}
			tc.setupMock(mockClient)

			service := &SDPSepServicesIntegrationTests{
				HTTPClient: mockClient,
				SDPBaseURL: "https://test.stellar.local:8000",
			}

			ctx := context.Background()
			err := service.CompleteReceiverRegistration(ctx, tc.sep24Token, tc.registrationData)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestSDPSepServicesIntegrationTests_Configuration(t *testing.T) {
	t.Parallel()

	receiverKP, _ := createTestKeypairs()

	service := &SDPSepServicesIntegrationTests{
		SDPBaseURL:                "https://test.stellar.local:8000",
		TenantName:                "test",
		ReceiverAccountPublicKey:  receiverKP.Address(),
		ReceiverAccountPrivateKey: receiverKP.Seed(),
		ClientDomain:              "test.stellar.local", 
		ClientDomainPrivateKey:    "",                   
		DisbursedAssetCode:        "USDC",
		HomeDomain:                "test.stellar.local:8000", 
	}

	// Test configuration validation
	t.Run("configuration validation", func(t *testing.T) {
		assert.NotEmpty(t, service.SDPBaseURL)
		assert.NotEmpty(t, service.ReceiverAccountPublicKey)
		assert.NotEmpty(t, service.ReceiverAccountPrivateKey)
		assert.NotEmpty(t, service.ClientDomain)
		assert.NotEmpty(t, service.DisbursedAssetCode)
	})

	// Test URL construction
	t.Run("URL construction", func(t *testing.T) {
		authURL, err := url.JoinPath(service.SDPBaseURL, "auth")
		assert.NoError(t, err)
		assert.Equal(t, "https://test.stellar.local:8000/auth", authURL)

		depositURL, err := url.JoinPath(service.SDPBaseURL, "sep24", "transactions", "deposit", "interactive")
		assert.NoError(t, err)
		assert.Equal(t, "https://test.stellar.local:8000/sep24/transactions/deposit/interactive", depositURL)
	})

	// Test keypair validation
	t.Run("keypair validation", func(t *testing.T) {
		_, err := keypair.ParseFull(service.ReceiverAccountPrivateKey)
		assert.NoError(t, err)

		_, err = keypair.ParseAddress(service.ReceiverAccountPublicKey)
		assert.NoError(t, err)
	})
}
