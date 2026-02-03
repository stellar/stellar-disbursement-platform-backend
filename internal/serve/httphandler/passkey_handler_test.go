package httphandler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/veraison/go-cose"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	servicesMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/wallet"
	walletMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/wallet/mocks"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
)

func generateCOSEPublicKey(t *testing.T) []byte {
	x := make([]byte, 32)
	y := make([]byte, 32)
	for i := range x {
		x[i] = byte(i)
		y[i] = byte(i + 32)
	}

	key, err := cose.NewKeyEC2(cose.AlgorithmES256, x, y, nil)
	require.NoError(t, err)

	coseBytes, err := key.MarshalCBOR()
	require.NoError(t, err)

	return coseBytes
}

func Test_StartPasskeyRegistrationRequest_Validate(t *testing.T) {
	testCases := []struct {
		name      string
		request   StartPasskeyRegistrationRequest
		wantError bool
	}{
		{
			name: "valid request",
			request: StartPasskeyRegistrationRequest{
				Token: "valid-token",
			},
			wantError: false,
		},
		{
			name: "empty token",
			request: StartPasskeyRegistrationRequest{
				Token: "",
			},
			wantError: true,
		},
		{
			name: "whitespace only token",
			request: StartPasskeyRegistrationRequest{
				Token: "   ",
			},
			wantError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.wantError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func Test_PasskeyHandler_StartPasskeyRegistration(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	requestBody, err := json.Marshal(StartPasskeyRegistrationRequest{
		Token: "valid-token",
	})
	require.NoError(t, err)
	ctx := context.Background()

	expectedCreation := &protocol.CredentialCreation{
		Response: protocol.PublicKeyCredentialCreationOptions{
			Challenge: []byte("test-challenge"),
		},
	}

	mockWebAuthnService.
		On("StartPasskeyRegistration", mock.Anything, "valid-token").
		Return(expectedCreation, nil).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/registration/start", strings.NewReader(string(requestBody)))
	require.NoError(t, err)
	http.HandlerFunc(handler.StartPasskeyRegistration).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Result().StatusCode)

	var respBody protocol.CredentialCreation
	err = json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)
	assert.Equal(t, expectedCreation.Response.Challenge, respBody.Response.Challenge)
}

func Test_PasskeyHandler_StartPasskeyRegistration_ValidationErrors(t *testing.T) {
	errorCases := []struct {
		name        string
		requestBody string
		expectField string
	}{
		{
			name:        "empty token",
			requestBody: `{"token": ""}`,
			expectField: "token",
		},
		{
			name:        "whitespace only token",
			requestBody: `{"token": "   "}`,
			expectField: "token",
		},
		{
			name:        "missing token field",
			requestBody: `{}`,
			expectField: "token",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
			mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
			mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
			handler := PasskeyHandler{
				WebAuthnService:       mockWebAuthnService,
				WalletJWTManager:      mockJWTManager,
				EmbeddedWalletService: mockEmbeddedWalletService,
			}

			rr := httptest.NewRecorder()

			req, err := http.NewRequest(http.MethodPost, "/embedded-wallets/passkey/registration/start", strings.NewReader(tc.requestBody))
			require.NoError(t, err)
			http.HandlerFunc(handler.StartPasskeyRegistration).ServeHTTP(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
			var errResp map[string]any
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&errResp))
			fields := errResp["extras"].(map[string]any)
			assert.Contains(t, fields, tc.expectField)
		})
	}
}

func Test_PasskeyHandler_StartPasskeyRegistration_ServiceErrors(t *testing.T) {
	errorCases := []struct {
		name               string
		serviceError       error
		expectedStatusCode int
	}{
		{
			name:               "invalid token",
			serviceError:       wallet.ErrInvalidToken,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "wallet already exists",
			serviceError:       wallet.ErrWalletAlreadyExists,
			expectedStatusCode: http.StatusConflict,
		},
		{
			name:               "unexpected error",
			serviceError:       errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
			mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
			mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
			handler := PasskeyHandler{
				WebAuthnService:       mockWebAuthnService,
				WalletJWTManager:      mockJWTManager,
				EmbeddedWalletService: mockEmbeddedWalletService,
			}

			rr := httptest.NewRecorder()
			requestBody, err := json.Marshal(StartPasskeyRegistrationRequest{
				Token: "test-token",
			})
			require.NoError(t, err)
			ctx := context.Background()

			mockWebAuthnService.
				On("StartPasskeyRegistration", mock.Anything, "test-token").
				Return(nil, tc.serviceError).
				Once()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/registration/start", strings.NewReader(string(requestBody)))
			require.NoError(t, err)
			http.HandlerFunc(handler.StartPasskeyRegistration).ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Result().StatusCode)
		})
	}
}

func Test_PasskeyHandler_FinishPasskeyRegistration(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	tenantID := "test-tenant-id"
	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

	mockCredential := &webauthn.Credential{
		ID:        []byte("test-credential-id"),
		PublicKey: generateCOSEPublicKey(t),
	}

	mockWebAuthnService.
		On("FinishPasskeyRegistration", mock.Anything, "valid-token", mock.AnythingOfType("*http.Request")).
		Return(mockCredential, nil).
		Once()

	expectedCredentialID := base64.RawURLEncoding.EncodeToString(mockCredential.ID)
	mockJWTManager.
		On("GenerateToken", mock.Anything, tenantID, expectedCredentialID, "", mock.AnythingOfType("time.Time")).
		Return("jwt-token", nil).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/registration/finish?token=valid-token", nil)
	require.NoError(t, err)
	http.HandlerFunc(handler.FinishPasskeyRegistration).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Result().StatusCode)

	var respBody PasskeyRegistrationResponse
	err = json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)
	assert.Equal(t, "jwt-token", respBody.Token)
	assert.NotEmpty(t, respBody.CredentialID)
	assert.NotEmpty(t, respBody.PublicKey)
	assert.Equal(t, expectedCredentialID, respBody.CredentialID)
}

func Test_PasskeyHandler_FinishPasskeyRegistration_MissingTokenQueryParam(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/registration/finish", nil)
	require.NoError(t, err)
	http.HandlerFunc(handler.FinishPasskeyRegistration).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_PasskeyHandler_FinishPasskeyRegistration_ServiceErrors(t *testing.T) {
	errorCases := []struct {
		name               string
		serviceError       error
		expectedStatusCode int
	}{
		{
			name:               "invalid token",
			serviceError:       wallet.ErrInvalidToken,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "wallet already exists",
			serviceError:       wallet.ErrWalletAlreadyExists,
			expectedStatusCode: http.StatusConflict,
		},
		{
			name:               "session not found",
			serviceError:       wallet.ErrSessionNotFound,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "session type mismatch",
			serviceError:       wallet.ErrSessionTypeMismatch,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "challenge mismatch",
			serviceError:       protocol.ErrChallengeMismatch,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "verification error",
			serviceError:       protocol.ErrVerification,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "unexpected error",
			serviceError:       errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
			mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
			mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
			handler := PasskeyHandler{
				WebAuthnService:       mockWebAuthnService,
				WalletJWTManager:      mockJWTManager,
				EmbeddedWalletService: mockEmbeddedWalletService,
			}

			rr := httptest.NewRecorder()
			ctx := context.Background()

			mockWebAuthnService.
				On("FinishPasskeyRegistration", mock.Anything, "test-token", mock.AnythingOfType("*http.Request")).
				Return(nil, tc.serviceError).
				Once()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/registration/finish?token=test-token", nil)
			require.NoError(t, err)
			http.HandlerFunc(handler.FinishPasskeyRegistration).ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Result().StatusCode)
		})
	}
}

func Test_PasskeyHandler_StartPasskeyAuthentication(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	expectedAssertion := &protocol.CredentialAssertion{
		Response: protocol.PublicKeyCredentialRequestOptions{
			Challenge: []byte("test-challenge"),
		},
	}

	mockWebAuthnService.
		On("StartPasskeyAuthentication", mock.Anything).
		Return(expectedAssertion, nil).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/start", nil)
	require.NoError(t, err)
	http.HandlerFunc(handler.StartPasskeyAuthentication).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Result().StatusCode)

	var respBody protocol.CredentialAssertion
	err = json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)
	assert.Equal(t, expectedAssertion.Response.Challenge, respBody.Response.Challenge)
}

func Test_PasskeyHandler_StartPasskeyAuthentication_InternalError(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	mockWebAuthnService.
		On("StartPasskeyAuthentication", mock.Anything).
		Return(nil, errors.New("database error")).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/start", nil)
	require.NoError(t, err)
	http.HandlerFunc(handler.StartPasskeyAuthentication).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
}

func Test_PasskeyHandler_FinishPasskeyAuthentication(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	tenantID := "test-tenant-id"
	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

	mockEmbeddedWallet := &data.EmbeddedWallet{
		Token:           "wallet-token",
		ContractAddress: "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX",
		CredentialID:    "test-credential-id",
		WalletStatus:    data.PendingWalletStatus,
	}

	mockWebAuthnService.
		On("FinishPasskeyAuthentication", mock.Anything, mock.AnythingOfType("*http.Request")).
		Return(mockEmbeddedWallet, nil).
		Once()

	mockJWTManager.
		On("GenerateToken", mock.Anything, tenantID, mockEmbeddedWallet.CredentialID, mockEmbeddedWallet.ContractAddress, mock.AnythingOfType("time.Time")).
		Return("jwt-token", nil).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/finish", nil)
	require.NoError(t, err)
	http.HandlerFunc(handler.FinishPasskeyAuthentication).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Result().StatusCode)

	var respBody PasskeyAuthenticationResponse
	err = json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)
	assert.Equal(t, "jwt-token", respBody.Token)
	assert.Equal(t, mockEmbeddedWallet.CredentialID, respBody.CredentialID)
	assert.Equal(t, mockEmbeddedWallet.ContractAddress, respBody.ContractAddress)
}

func Test_PasskeyHandler_FinishPasskeyAuthentication_ServiceErrors(t *testing.T) {
	errorCases := []struct {
		name               string
		serviceError       error
		expectedStatusCode int
	}{
		{
			name:               "wallet not ready",
			serviceError:       wallet.ErrWalletNotReady,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "session not found",
			serviceError:       wallet.ErrSessionNotFound,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "session type mismatch",
			serviceError:       wallet.ErrSessionTypeMismatch,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "challenge mismatch",
			serviceError:       protocol.ErrChallengeMismatch,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "verification error",
			serviceError:       protocol.ErrVerification,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "unexpected error",
			serviceError:       errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
			mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
			mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
			handler := PasskeyHandler{
				WebAuthnService:       mockWebAuthnService,
				WalletJWTManager:      mockJWTManager,
				EmbeddedWalletService: mockEmbeddedWalletService,
			}

			rr := httptest.NewRecorder()
			ctx := context.Background()

			mockWebAuthnService.
				On("FinishPasskeyAuthentication", mock.Anything, mock.AnythingOfType("*http.Request")).
				Return(nil, tc.serviceError).
				Once()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/finish", nil)
			require.NoError(t, err)
			http.HandlerFunc(handler.FinishPasskeyAuthentication).ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Result().StatusCode)
		})
	}
}

func Test_PasskeyHandler_FinishPasskeyAuthentication_JWTGenerationError(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	tenantID := "test-tenant-id"
	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

	mockEmbeddedWallet := &data.EmbeddedWallet{
		Token:           "wallet-token",
		ContractAddress: "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX",
		CredentialID:    "test-credential-id",
		WalletStatus:    data.PendingWalletStatus,
	}

	mockWebAuthnService.
		On("FinishPasskeyAuthentication", mock.Anything, mock.AnythingOfType("*http.Request")).
		Return(mockEmbeddedWallet, nil).
		Once()

	mockJWTManager.
		On("GenerateToken", mock.Anything, tenantID, mockEmbeddedWallet.CredentialID, mockEmbeddedWallet.ContractAddress, mock.AnythingOfType("time.Time")).
		Return("", errors.New("JWT signing error")).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/finish", nil)
	require.NoError(t, err)
	http.HandlerFunc(handler.FinishPasskeyAuthentication).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
}

func Test_PasskeyHandler_FinishPasskeyAuthentication_TokenExpiration(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	tenantID := "test-tenant-id"
	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

	mockEmbeddedWallet := &data.EmbeddedWallet{
		Token:           "wallet-token",
		ContractAddress: "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX",
		CredentialID:    "test-credential-id",
		WalletStatus:    data.PendingWalletStatus,
	}

	mockWebAuthnService.
		On("FinishPasskeyAuthentication", mock.Anything, mock.AnythingOfType("*http.Request")).
		Return(mockEmbeddedWallet, nil).
		Once()

	var capturedExpiresAt time.Time
	mockJWTManager.
		On("GenerateToken", mock.Anything, tenantID, mockEmbeddedWallet.CredentialID, mockEmbeddedWallet.ContractAddress, mock.AnythingOfType("time.Time")).
		Run(func(args mock.Arguments) {
			capturedExpiresAt = args.Get(4).(time.Time)
		}).
		Return("jwt-token", nil).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/finish", nil)
	require.NoError(t, err)
	http.HandlerFunc(handler.FinishPasskeyAuthentication).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Result().StatusCode)

	expectedExpiry := time.Now().Add(WalletTokenExpiration)
	assert.WithinDuration(t, expectedExpiry, capturedExpiresAt, 5*time.Second)
}

func Test_RefreshTokenRequest_Validate(t *testing.T) {
	testCases := []struct {
		name      string
		request   RefreshTokenRequest
		wantError bool
	}{
		{
			name: "valid request",
			request: RefreshTokenRequest{
				Token: "valid-token",
			},
			wantError: false,
		},
		{
			name: "empty token",
			request: RefreshTokenRequest{
				Token: "",
			},
			wantError: true,
		},
		{
			name: "whitespace only token",
			request: RefreshTokenRequest{
				Token: "   ",
			},
			wantError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.wantError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func Test_PasskeyHandler_RefreshToken(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	requestBody, err := json.Marshal(RefreshTokenRequest{
		Token: "valid-token",
	})
	require.NoError(t, err)
	tenantID := "test-tenant-id"
	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

	contractAddress := "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX"
	credentialID := "test-credential-id"

	mockJWTManager.
		On("ValidateToken", mock.Anything, "valid-token").
		Return(credentialID, contractAddress, tenantID, nil).
		Once()

	var capturedExpiresAt time.Time
	mockJWTManager.
		On("GenerateToken", mock.Anything, tenantID, credentialID, contractAddress, mock.AnythingOfType("time.Time")).
		Run(func(args mock.Arguments) {
			capturedExpiresAt = args.Get(4).(time.Time)
		}).
		Return("refreshed-token", nil).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/refresh", strings.NewReader(string(requestBody)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	http.HandlerFunc(handler.RefreshToken).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Result().StatusCode)

	var respBody RefreshTokenResponse
	err = json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)
	assert.Equal(t, "refreshed-token", respBody.Token)

	expectedExpiry := time.Now().Add(WalletTokenExpiration)
	assert.WithinDuration(t, expectedExpiry, capturedExpiresAt, 5*time.Second)
}

func Test_PasskeyHandler_RefreshToken_TenantMismatch(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	requestBody, err := json.Marshal(RefreshTokenRequest{
		Token: "valid-token",
	})
	require.NoError(t, err)

	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: "request-tenant"})

	mockJWTManager.
		On("ValidateToken", mock.Anything, "valid-token").
		Return("credential-id", "contract-address", "token-tenant", nil).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/refresh", strings.NewReader(string(requestBody)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	http.HandlerFunc(handler.RefreshToken).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Result().StatusCode)
}

func Test_PasskeyHandler_RefreshToken_InvalidRequestBody(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/refresh", strings.NewReader("invalid-json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	http.HandlerFunc(handler.RefreshToken).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_PasskeyHandler_RefreshToken_ValidationError(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	requestBody, err := json.Marshal(RefreshTokenRequest{
		Token: "",
	})
	require.NoError(t, err)
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/refresh", strings.NewReader(string(requestBody)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	http.HandlerFunc(handler.RefreshToken).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_PasskeyHandler_RefreshToken_ValidateTokenErrors(t *testing.T) {
	errorCases := []struct {
		name               string
		validationError    error
		expectedStatusCode int
	}{
		{
			name:               "expired token",
			validationError:    wallet.ErrExpiredWalletToken,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "invalid token",
			validationError:    wallet.ErrInvalidWalletToken,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "missing sub claim",
			validationError:    wallet.ErrMissingSubClaim,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "missing tenant claim",
			validationError:    wallet.ErrMissingTenantClaim,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "unexpected error",
			validationError:    errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
			mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
			mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
			handler := PasskeyHandler{
				WebAuthnService:       mockWebAuthnService,
				WalletJWTManager:      mockJWTManager,
				EmbeddedWalletService: mockEmbeddedWalletService,
			}

			rr := httptest.NewRecorder()
			requestBody, err := json.Marshal(RefreshTokenRequest{
				Token: "test-token",
			})
			require.NoError(t, err)
			tenantID := "test-tenant-id"
			ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

			mockJWTManager.
				On("ValidateToken", mock.Anything, "test-token").
				Return("", "", "", tc.validationError).
				Once()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/refresh", strings.NewReader(string(requestBody)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			http.HandlerFunc(handler.RefreshToken).ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Result().StatusCode)
		})
	}
}

func Test_PasskeyHandler_RefreshToken_GenerateTokenErrors(t *testing.T) {
	errorCases := []struct {
		name               string
		generateError      error
		expectedStatusCode int
	}{
		{
			name:               "signing error",
			generateError:      errors.New("signing error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
			mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
			mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
			handler := PasskeyHandler{
				WebAuthnService:       mockWebAuthnService,
				WalletJWTManager:      mockJWTManager,
				EmbeddedWalletService: mockEmbeddedWalletService,
			}

			rr := httptest.NewRecorder()
			requestBody, err := json.Marshal(RefreshTokenRequest{
				Token: "test-token",
			})
			require.NoError(t, err)
			tenantID := "test-tenant-id"
			ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

			contractAddress := "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX"
			credentialID := "test-credential-id"

			mockJWTManager.
				On("ValidateToken", mock.Anything, "test-token").
				Return(credentialID, contractAddress, tenantID, nil).
				Once()

			mockJWTManager.
				On("GenerateToken", mock.Anything, tenantID, credentialID, contractAddress, mock.AnythingOfType("time.Time")).
				Return("", tc.generateError).
				Once()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/refresh", strings.NewReader(string(requestBody)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			http.HandlerFunc(handler.RefreshToken).ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Result().StatusCode)
		})
	}
}

func Test_PasskeyHandler_RefreshToken_UpdatesContractAddress(t *testing.T) {
	mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
	mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	handler := PasskeyHandler{
		WebAuthnService:       mockWebAuthnService,
		WalletJWTManager:      mockJWTManager,
		EmbeddedWalletService: mockEmbeddedWalletService,
	}

	rr := httptest.NewRecorder()
	requestBody, err := json.Marshal(RefreshTokenRequest{
		Token: "valid-token",
	})
	require.NoError(t, err)
	tenantID := "test-tenant-id"
	ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

	credentialID := "test-credential-id"
	oldContractAddress := ""
	newContractAddress := "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX"

	mockJWTManager.
		On("ValidateToken", mock.Anything, "valid-token").
		Return(credentialID, oldContractAddress, tenantID, nil).
		Once()

	mockEmbeddedWalletService.
		On("GetWalletByCredentialID", mock.Anything, credentialID).
		Return(&data.EmbeddedWallet{
			CredentialID:    credentialID,
			ContractAddress: newContractAddress,
		}, nil).
		Once()

	mockJWTManager.
		On("GenerateToken", mock.Anything, tenantID, credentialID, newContractAddress, mock.AnythingOfType("time.Time")).
		Return("refreshed-token-with-address", nil).
		Once()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/refresh", strings.NewReader(string(requestBody)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	http.HandlerFunc(handler.RefreshToken).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Result().StatusCode)

	var respBody RefreshTokenResponse
	err = json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)
	assert.Equal(t, "refreshed-token-with-address", respBody.Token)
}

func Test_PasskeyHandler_RefreshToken_WalletLookupError(t *testing.T) {
	errorCases := []struct {
		name               string
		lookupError        error
		expectedStatusCode int
	}{
		{
			name:               "invalid credential ID",
			lookupError:        services.ErrInvalidCredentialID,
			expectedStatusCode: http.StatusUnauthorized,
		},
		{
			name:               "database error",
			lookupError:        errors.New("database error"),
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			mockWebAuthnService := walletMocks.NewMockWebAuthnService(t)
			mockEmbeddedWalletService := servicesMocks.NewMockEmbeddedWalletService(t)
			mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
			handler := PasskeyHandler{
				WebAuthnService:       mockWebAuthnService,
				WalletJWTManager:      mockJWTManager,
				EmbeddedWalletService: mockEmbeddedWalletService,
			}

			rr := httptest.NewRecorder()
			requestBody, err := json.Marshal(RefreshTokenRequest{
				Token: "valid-token",
			})
			require.NoError(t, err)
			tenantID := "test-tenant-id"
			ctx := sdpcontext.SetTenantInContext(context.Background(), &schema.Tenant{ID: tenantID})

			credentialID := "test-credential-id"

			mockJWTManager.
				On("ValidateToken", mock.Anything, "valid-token").
				Return(credentialID, "", tenantID, nil).
				Once()

			mockEmbeddedWalletService.
				On("GetWalletByCredentialID", mock.Anything, credentialID).
				Return((*data.EmbeddedWallet)(nil), tc.lookupError).
				Once()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallets/passkey/authentication/refresh", strings.NewReader(string(requestBody)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			http.HandlerFunc(handler.RefreshToken).ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatusCode, rr.Result().StatusCode)
		})
	}
}
