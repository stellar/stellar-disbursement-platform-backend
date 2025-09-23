package httphandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/services"
)

func Test_SEP10Handler_GetChallenge(t *testing.T) {
	t.Run("✅successful challenge creation", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		expectedChallenge := &services.ChallengeResponse{
			Transaction:       "AAAA...",
			NetworkPassphrase: "Test SDF Network ; September 2015",
		}

		mockService.On("CreateChallenge", mock.Anything, services.ChallengeRequest{
			Account:      "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			Memo:         "123",
			HomeDomain:   "stellar.org",
			ClientDomain: "client.stellar.org",
		}).Return(expectedChallenge, nil)

		r := chi.NewRouter()
		r.Get("/auth", handler.GetChallenge)

		req := httptest.NewRequest(http.MethodGet, "/auth?account=GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF&memo=123&home_domain=stellar.org&client_domain=client.stellar.org", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

		var response services.ChallengeResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, expectedChallenge.Transaction, response.Transaction)
		assert.Equal(t, expectedChallenge.NetworkPassphrase, response.NetworkPassphrase)
	})

	t.Run("❌missing account parameter", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		r := chi.NewRouter()
		r.Get("/auth", handler.GetChallenge)

		req := httptest.NewRequest(http.MethodGet, "/auth?memo=123&home_domain=stellar.org", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "account is required")
	})

	t.Run("❌invalid account format", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		r := chi.NewRouter()
		r.Get("/auth", handler.GetChallenge)

		req := httptest.NewRequest(http.MethodGet, "/auth?account=invalid-account&client_domain=client.stellar.org", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid account not a valid ed25519 public key")
	})

	t.Run("❌invalid memo type", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		r := chi.NewRouter()
		r.Get("/auth", handler.GetChallenge)

		req := httptest.NewRequest(http.MethodGet, "/auth?account=GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF&memo=invalid-memo&client_domain=client.stellar.org", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid memo")
	})

	t.Run("❌service error", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		mockService.On("CreateChallenge", mock.Anything, services.ChallengeRequest{
			Account:      "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
			ClientDomain: "client.stellar.org",
		}).Return(nil, fmt.Errorf("service error"))

		r := chi.NewRouter()
		r.Get("/auth", handler.GetChallenge)

		req := httptest.NewRequest(http.MethodGet, "/auth?account=GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF&client_domain=client.stellar.org", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to create challenge")
	})
}

func Test_SEP10Handler_PostChallenge(t *testing.T) {
	t.Run("✅successful challenge validation with JSON", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		expectedResponse := &services.ValidationResponse{
			Token: "eyJ...",
		}

		reqBody := services.ValidationRequest{
			Transaction: "AAAA...",
		}
		reqBodyBytes, _ := json.Marshal(reqBody)

		mockService.On("ValidateChallenge", mock.Anything, reqBody).Return(expectedResponse, nil)

		r := chi.NewRouter()
		r.Post("/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/auth", bytes.NewBuffer(reqBodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

		var response services.ValidationResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, expectedResponse.Token, response.Token)
	})

	t.Run("✅successful challenge validation with form data", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		expectedResponse := &services.ValidationResponse{
			Token: "eyJ...",
		}

		reqBody := services.ValidationRequest{
			Transaction: "AAAA...",
		}

		mockService.On("ValidateChallenge", mock.Anything, reqBody).Return(expectedResponse, nil)

		r := chi.NewRouter()
		r.Post("/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/auth", bytes.NewBufferString("transaction=AAAA..."))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

		var response services.ValidationResponse
		err := json.NewDecoder(w.Body).Decode(&response)
		require.NoError(t, err)
		assert.Equal(t, expectedResponse.Token, response.Token)
	})

	t.Run("❌unsupported content type", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		r := chi.NewRouter()
		r.Post("/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/auth", bytes.NewBufferString("data"))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "unsupported content type")
	})

	t.Run("❌invalid JSON body", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		r := chi.NewRouter()
		r.Post("/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/auth", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid request body")
	})

	t.Run("❌missing transaction", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		r := chi.NewRouter()
		r.Post("/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/auth", bytes.NewBufferString("other=data"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "transaction is required")
	})

	t.Run("❌service validation error", func(t *testing.T) {
		mockService := services.NewMockSEP10Service(t)
		handler := SEP10Handler{SEP10Service: mockService}

		reqBody := services.ValidationRequest{
			Transaction: "AAAA...",
		}

		mockService.On("ValidateChallenge", mock.Anything, reqBody).Return(nil, fmt.Errorf("validation failed"))

		r := chi.NewRouter()
		r.Post("/auth", handler.PostChallenge)

		req := httptest.NewRequest(http.MethodPost, "/auth", bytes.NewBufferString("transaction=AAAA..."))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "challenge validation failed")
	})
}

func Test_SEP10Handler_validateChallengeRequest(t *testing.T) {
	testCases := []struct {
		name          string
		request       services.ChallengeRequest
		expectedError string
	}{
		{
			name: "✅valid request",
			request: services.ChallengeRequest{
				Account:      "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
				Memo:         "123",
				HomeDomain:   "stellar.org",
				ClientDomain: "client.stellar.org",
			},
			expectedError: "",
		},
		{
			name: "✅valid request without memo",
			request: services.ChallengeRequest{
				Account:      "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
				HomeDomain:   "stellar.org",
				ClientDomain: "client.stellar.org",
			},
			expectedError: "",
		},
		{
			name: "❌missing account",
			request: services.ChallengeRequest{
				Memo:         "123",
				HomeDomain:   "stellar.org",
				ClientDomain: "client.stellar.org",
			},
			expectedError: "account is required",
		},
		{
			name: "❌invalid account format",
			request: services.ChallengeRequest{
				Account:      "invalid-account",
				Memo:         "123",
				HomeDomain:   "stellar.org",
				ClientDomain: "client.stellar.org",
			},
			expectedError: "invalid account not a valid ed25519 public key",
		},
		{
			name: "❌invalid memo type",
			request: services.ChallengeRequest{
				Account:      "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF",
				Memo:         "invalid-memo",
				HomeDomain:   "stellar.org",
				ClientDomain: "client.stellar.org",
			},
			expectedError: "invalid memo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			}
		})
	}
}
