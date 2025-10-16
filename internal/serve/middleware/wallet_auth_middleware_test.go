package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/wallet"
	walletMocks "github.com/stellar/stellar-disbursement-platform-backend/internal/wallet/mocks"
)

func Test_WalletAuthMiddleware_SuccessfulAuthentication(t *testing.T) {
	contractAddress := "CBGTG3VGUMVDZE6O4CRZ2LBCFP7O5XY2VQQQU7AVXLVDQHZLVQFRMHKX"
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	mockJWTManager.
		On("ValidateToken", mock.Anything, "valid-token").
		Return(contractAddress, nil).
		Once()

	var capturedContractAddress string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedContractAddress, err = sdpcontext.GetWalletContractAddressFromContext(r.Context())
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	})

	r := chi.NewRouter()
	r.Use(WalletAuthMiddleware(mockJWTManager))
	r.Get("/test", h)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, contractAddress, capturedContractAddress)
}

func Test_WalletAuthMiddleware_MissingAuthorizationHeader(t *testing.T) {
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)

	r := chi.NewRouter()
	r.Use(WalletAuthMiddleware(mockJWTManager))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func Test_WalletAuthMiddleware_EmptyAuthorizationHeader(t *testing.T) {
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)

	r := chi.NewRouter()
	r.Use(WalletAuthMiddleware(mockJWTManager))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func Test_WalletAuthMiddleware_InvalidAuthorizationFormat_NoBearer(t *testing.T) {
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)

	r := chi.NewRouter()
	r.Use(WalletAuthMiddleware(mockJWTManager))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "valid-token")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func Test_WalletAuthMiddleware_InvalidAuthorizationFormat_WrongPrefix(t *testing.T) {
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)

	r := chi.NewRouter()
	r.Use(WalletAuthMiddleware(mockJWTManager))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic valid-token")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func Test_WalletAuthMiddleware_InvalidToken(t *testing.T) {
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	mockJWTManager.
		On("ValidateToken", mock.Anything, "invalid-token").
		Return("", wallet.ErrInvalidWalletToken).
		Once()

	r := chi.NewRouter()
	r.Use(WalletAuthMiddleware(mockJWTManager))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func Test_WalletAuthMiddleware_ExpiredToken(t *testing.T) {
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	mockJWTManager.
		On("ValidateToken", mock.Anything, "expired-token").
		Return("", wallet.ErrExpiredWalletToken).
		Once()

	r := chi.NewRouter()
	r.Use(WalletAuthMiddleware(mockJWTManager))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func Test_WalletAuthMiddleware_UnexpectedValidationError(t *testing.T) {
	mockJWTManager := walletMocks.NewMockWalletJWTManager(t)
	mockJWTManager.
		On("ValidateToken", mock.Anything, "error-token").
		Return("", errors.New("unexpected database error")).
		Once()

	r := chi.NewRouter()
	r.Use(WalletAuthMiddleware(mockJWTManager))
	r.Get("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer error-token")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
