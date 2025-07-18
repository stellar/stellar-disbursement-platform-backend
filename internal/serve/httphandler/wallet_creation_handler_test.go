package httphandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/services/mocks"
)

func Test_WalletCreationHandler_CreateWallet(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:        "123",
		PublicKey:    "04f5",
		CredentialID: "test-credential-id",
	})
	ctx := context.Background()

	walletService.On("CreateWallet", mock.Anything, "123", "04f5", "test-credential-id").Return(nil)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusAccepted, rr.Result().StatusCode)
	var respBody WalletResponse
	err := json.Unmarshal(rr.Body.Bytes(), &respBody)
	require.NoError(t, err)

	assert.Equal(t, data.PendingWalletStatus, respBody.Status)
	assert.Empty(t, respBody.ContractAddress)
}

func Test_WalletCreationHandler_CreateWallet_ValidationErrors(t *testing.T) {
	errorCases := []struct {
		name          string
		requestBody   CreateWalletRequest
		expectedField string
	}{
		{
			name: "empty token",
			requestBody: CreateWalletRequest{
				Token:        "",
				PublicKey:    "04f5",
				CredentialID: "test-credential-id",
			},
			expectedField: "token",
		},
		{
			name: "empty public key",
			requestBody: CreateWalletRequest{
				Token:        "123",
				PublicKey:    "",
				CredentialID: "test-credential-id",
			},
			expectedField: "public_key",
		},
		{
			name: "invalid public key",
			requestBody: CreateWalletRequest{
				Token:        "123",
				PublicKey:    "invalid_key",
				CredentialID: "test-credential-id",
			},
			expectedField: "public_key",
		},
		{
			name: "empty credential id",
			requestBody: CreateWalletRequest{
				Token:        "123",
				PublicKey:    "04f5",
				CredentialID: "",
			},
			expectedField: "credential_id",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			walletService := mocks.NewMockEmbeddedWalletService(t)
			handler := WalletCreationHandler{
				EmbeddedWalletService: walletService,
			}

			rr := httptest.NewRecorder()
			requestBody, _ := json.Marshal(tc.requestBody)

			req, _ := http.NewRequest(http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
			http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
			var errResp map[string]any
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&errResp))
			fields := errResp["extras"].(map[string]any)
			assert.Contains(t, fields, tc.expectedField)
		})
	}
}

func Test_WalletCreationHandler_CreateWallet_InternalError(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:        "123",
		PublicKey:    "04f5",
		CredentialID: "test-credential-id",
	})
	ctx := context.Background()

	walletService.On("CreateWallet", mock.Anything, "123", "04f5", "test-credential-id").Return(errors.New("foobar"))

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_CreateWallet_InvalidToken(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:        "123",
		PublicKey:    "04f5",
		CredentialID: "test-credential-id",
	})
	ctx := context.Background()

	walletService.On("CreateWallet", mock.Anything, "123", "04f5", "test-credential-id").Return(services.ErrInvalidToken)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_CreateWallet_InvalidStatus(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:        "123",
		PublicKey:    "04f5",
		CredentialID: "test-credential-id",
	})
	ctx := context.Background()

	walletService.On("CreateWallet", mock.Anything, "123", "04f5", "test-credential-id").Return(services.ErrCreateWalletInvalidStatus)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_CreateWallet_CredentialIDConflict(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(CreateWalletRequest{
		Token:        "123",
		PublicKey:    "04f5",
		CredentialID: "duplicate-credential-id",
	})
	ctx := context.Background()

	walletService.On("CreateWallet", mock.Anything, "123", "04f5", "duplicate-credential-id").Return(services.ErrCredentialIDAlreadyExists)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "/embedded-wallet/", strings.NewReader(string(requestBody)))
	http.HandlerFunc(handler.CreateWallet).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Result().StatusCode)

	responseBody := rr.Body.String()
	assert.Contains(t, responseBody, "Credential ID already exists")
}

func Test_WalletCreationHandler_GetWallet(t *testing.T) {
	testCases := []struct {
		name             string
		credentialID     string
		setupMock        func(*mocks.MockEmbeddedWalletService)
		expectedStatus   int
		expectedResponse *WalletResponse
	}{
		{
			name:         "successfully gets wallet",
			credentialID: "test-credential-id",
			setupMock: func(walletService *mocks.MockEmbeddedWalletService) {
				walletService.On("GetWalletByCredentialID", mock.Anything, "test-credential-id").Return(&data.EmbeddedWallet{
					ContractAddress: "contract-address",
					WalletStatus:    data.SuccessWalletStatus,
					CredentialID:    "test-credential-id",
				}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedResponse: &WalletResponse{
				ContractAddress: "contract-address",
				Status:          data.SuccessWalletStatus,
			},
		},
		{
			name:         "returns error for invalid credential ID",
			credentialID: "invalid-id",
			setupMock: func(walletService *mocks.MockEmbeddedWalletService) {
				walletService.On("GetWalletByCredentialID", mock.Anything, "invalid-id").Return(nil, services.ErrInvalidCredentialID)
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:         "returns error for internal service error",
			credentialID: "test-error-id",
			setupMock: func(walletService *mocks.MockEmbeddedWalletService) {
				walletService.On("GetWalletByCredentialID", mock.Anything, "test-error-id").Return(nil, errors.New("internal error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			walletService := mocks.NewMockEmbeddedWalletService(t)
			handler := WalletCreationHandler{
				EmbeddedWalletService: walletService,
			}

			tc.setupMock(walletService)

			r := chi.NewRouter()
			r.Get("/embedded-wallets/{credentialID}", handler.GetWallet)

			rr := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/embedded-wallets/"+tc.credentialID, nil)

			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Result().StatusCode)

			if tc.expectedResponse != nil {
				var respBody WalletResponse
				err := json.Unmarshal(rr.Body.Bytes(), &respBody)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedResponse.ContractAddress, respBody.ContractAddress)
				assert.Equal(t, tc.expectedResponse.Status, respBody.Status)
			}
		})
	}
}

func Test_WalletCreationHandler_GetWallet_EmptyCredentialID(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	r := chi.NewRouter()
	r.Get("/embedded-wallets/{credentialID}", handler.GetWallet)

	rr := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/embedded-wallets/ ", nil)

	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_GetWalletStatus(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	testCases := []struct {
		name           string
		token          string
		mockWallet     *data.EmbeddedWallet
		mockError      error
		expectedStatus int
		expectedBody   WalletStatusResponse
	}{
		{
			name:  "successful wallet status retrieval with email",
			token: "test-token-123",
			mockWallet: &data.EmbeddedWallet{
				Token:           "test-token-123",
				ContractAddress: "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAHHAGCKAA",
				WalletStatus:    data.SuccessWalletStatus,
				ReceiverContact: "user@example.com",
				ContactType:     data.ContactTypeEmail,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedBody: WalletStatusResponse{
				ContractAddress: "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQAHHAGCKAA",
				Status:          data.SuccessWalletStatus,
				ReceiverContact: "user@example.com",
				ContactType:     data.ContactTypeEmail,
			},
		},
		{
			name:  "successful wallet status retrieval with phone number",
			token: "test-token-456",
			mockWallet: &data.EmbeddedWallet{
				Token:           "test-token-456",
				ContractAddress: "",
				WalletStatus:    data.PendingWalletStatus,
				ReceiverContact: "+14155551234",
				ContactType:     data.ContactTypePhoneNumber,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedBody: WalletStatusResponse{
				ContractAddress: "",
				Status:          data.PendingWalletStatus,
				ReceiverContact: "+14155551234",
				ContactType:     data.ContactTypePhoneNumber,
			},
		},
		{
			name:           "invalid token",
			token:          "invalid-token",
			mockWallet:     nil,
			mockError:      services.ErrInvalidToken,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "internal server error",
			token:          "test-token-error",
			mockWallet:     nil,
			mockError:      errors.New("database connection failed"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			walletService.On("GetWalletByToken", mock.Anything, tc.token).Return(tc.mockWallet, tc.mockError).Once()

			r := chi.NewRouter()
			r.Get("/embedded-wallets/status/{token}", handler.GetWalletStatus)

			rr := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/embedded-wallets/status/"+tc.token, nil)

			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Result().StatusCode)

			if tc.expectedStatus == http.StatusOK {
				var resp WalletStatusResponse
				require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
				assert.Equal(t, tc.expectedBody, resp)
			}
		})
	}
}

func Test_WalletCreationHandler_GetWalletStatus_EmptyToken(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	r := chi.NewRouter()
	r.Get("/embedded-wallets/status/{token}", handler.GetWalletStatus)

	rr := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/embedded-wallets/status/ ", nil)

	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Result().StatusCode)
}

func Test_WalletCreationHandler_ResendInvite(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	testCases := []struct {
		name             string
		requestBody      ResendInviteRequest
		mockError        error
		expectedStatus   int
		expectedResponse *ResendInviteResponse
	}{
		{
			name: "successful resend invite for email",
			requestBody: ResendInviteRequest{
				ReceiverContact: "test@example.com",
				ContactType:     data.ContactTypeEmail,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedResponse: &ResendInviteResponse{
				Message: "Invitation queued for sending",
			},
		},
		{
			name: "successful resend invite for phone number",
			requestBody: ResendInviteRequest{
				ReceiverContact: "+14155555555",
				ContactType:     data.ContactTypePhoneNumber,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedResponse: &ResendInviteResponse{
				Message: "Invitation queued for sending",
			},
		},
		{
			name: "receiver not found",
			requestBody: ResendInviteRequest{
				ReceiverContact: "notfound@example.com",
				ContactType:     data.ContactTypeEmail,
			},
			mockError:      data.ErrRecordNotFound,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "internal server error",
			requestBody: ResendInviteRequest{
				ReceiverContact: "error@example.com",
				ContactType:     data.ContactTypeEmail,
			},
			mockError:      errors.New("database connection failed"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			walletService.On("ResendInvite", mock.Anything, tc.requestBody.ReceiverContact, string(tc.requestBody.ContactType)).Return(tc.mockError).Once()

			r := chi.NewRouter()
			r.Post("/embedded-wallets/resend-invite", handler.ResendInvite)

			rr := httptest.NewRecorder()
			requestBody, _ := json.Marshal(tc.requestBody)
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/embedded-wallets/resend-invite", strings.NewReader(string(requestBody)))

			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Result().StatusCode)

			if tc.expectedResponse != nil {
				var resp ResendInviteResponse
				require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
				assert.Equal(t, tc.expectedResponse.Message, resp.Message)
			}
		})
	}
}

func Test_WalletCreationHandler_ResendInvite_ValidationErrors(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	testCases := []struct {
		name           string
		requestBody    ResendInviteRequest
		expectedStatus int
		expectedField  string
	}{
		{
			name: "empty receiver contact",
			requestBody: ResendInviteRequest{
				ReceiverContact: "",
				ContactType:     data.ContactTypeEmail,
			},
			expectedStatus: http.StatusBadRequest,
			expectedField:  "receiver_contact",
		},
		{
			name: "invalid contact type",
			requestBody: ResendInviteRequest{
				ReceiverContact: "test@example.com",
				ContactType:     "INVALID_TYPE",
			},
			expectedStatus: http.StatusBadRequest,
			expectedField:  "contact_type",
		},
		{
			name: "invalid email format",
			requestBody: ResendInviteRequest{
				ReceiverContact: "invalid-email",
				ContactType:     data.ContactTypeEmail,
			},
			expectedStatus: http.StatusBadRequest,
			expectedField:  "receiver_contact",
		},
		{
			name: "invalid phone format",
			requestBody: ResendInviteRequest{
				ReceiverContact: "not-a-phone",
				ContactType:     data.ContactTypePhoneNumber,
			},
			expectedStatus: http.StatusBadRequest,
			expectedField:  "receiver_contact",
		},
		{
			name: "mismatched contact format",
			requestBody: ResendInviteRequest{
				ReceiverContact: "test@example.com",
				ContactType:     data.ContactTypePhoneNumber,
			},
			expectedStatus: http.StatusBadRequest,
			expectedField:  "receiver_contact",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Post("/embedded-wallets/resend-invite", handler.ResendInvite)

			rr := httptest.NewRecorder()
			requestBody, _ := json.Marshal(tc.requestBody)
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/embedded-wallets/resend-invite", strings.NewReader(string(requestBody)))

			r.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Result().StatusCode)

			var errorResp map[string]interface{}
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&errorResp))
			extras, ok := errorResp["extras"].(map[string]interface{})
			if ok {
				assert.Contains(t, extras, tc.expectedField)
			} else {
				assert.Contains(t, errorResp["error"].(string), tc.expectedField)
			}
		})
	}
}

func Test_WalletCreationHandler_ResendInvite_WalletStatusError(t *testing.T) {
	walletService := mocks.NewMockEmbeddedWalletService(t)
	handler := WalletCreationHandler{
		EmbeddedWalletService: walletService,
	}

	walletService.On("ResendInvite", mock.Anything, "test@example.com", "EMAIL").Return(services.ErrInvalidWalletStatus).Once()

	r := chi.NewRouter()
	r.Post("/embedded-wallets/resend-invite", handler.ResendInvite)

	rr := httptest.NewRecorder()
	requestBody, _ := json.Marshal(ResendInviteRequest{
		ReceiverContact: "test@example.com",
		ContactType:     data.ContactTypeEmail,
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/embedded-wallets/resend-invite", strings.NewReader(string(requestBody)))

	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Result().StatusCode)

	var errorResp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&errorResp))
	assert.Contains(t, errorResp["error"].(string), "Invitation already used")
}
