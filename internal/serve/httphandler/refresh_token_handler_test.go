package httphandler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_RefreshTokenHandler(t *testing.T) {
	jwtManagerMock := &auth.JWTManagerMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomJWTManagerOption(jwtManagerMock),
	)

	handler := &RefreshTokenHandler{AuthManager: authManager}
	url := "/refresh-token"

	ctx := context.Background()

	t.Run("returns Unauthorized error when no token is found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)

		http.HandlerFunc(handler.PostRefreshToken).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	t.Run("returns BadRequest when token is expired", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "mytoken").
			Return(false, nil).
			Once()

		http.HandlerFunc(handler.PostRefreshToken).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"token": "token is invalid"}}`, string(respBody))
	})

	t.Run("returns InternalServerError when AuthManager fails", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "mytoken").
			Return(false, errors.New("unexpected error")).
			Once()

		http.HandlerFunc(handler.PostRefreshToken).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Cannot refresh user token"}`, string(respBody))
	})

	t.Run("returns the refreshed token", func(t *testing.T) {
		ctx = context.WithValue(ctx, middleware.TokenContextKey, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "mytoken").
			Return(true, nil).
			Once().
			On("RefreshToken", req.Context(), "mytoken", mock.AnythingOfType("time.Time")).
			Return("myrefreshedtoken", nil).
			Once()

		http.HandlerFunc(handler.PostRefreshToken).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"token": "myrefreshedtoken"}`, string(respBody))
	})

	jwtManagerMock.AssertExpectations(t)
}
