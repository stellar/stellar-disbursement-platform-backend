package httphandler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func Test_RefreshTokenHandler(t *testing.T) {
	jwtManagerMock := &auth.JWTManagerMock{}
	authManager := auth.NewAuthManager(
		auth.WithCustomJWTManagerOption(jwtManagerMock),
	)
	tenantManagerMock := &tenant.TenantManagerMock{}

	handler := &RefreshTokenHandler{AuthManager: authManager, TenantManager: tenantManagerMock}
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
		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)

		// Invalid token: the tenant lookup is skipped and RefreshToken renders the response.
		jwtManagerMock.
			On("ValidateToken", req.Context(), "mytoken").
			Return(false, nil).
			Times(2)

		http.HandlerFunc(handler.PostRefreshToken).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error": "The request was invalid in some way.", "extras": {"token": "token is invalid"}}`, string(respBody))
	})

	t.Run("returns InternalServerError when AuthManager fails", func(t *testing.T) {
		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "mytoken").
			Return(false, errors.New("unexpected error")).
			Times(2)

		http.HandlerFunc(handler.PostRefreshToken).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Cannot refresh user token"}`, string(respBody))
	})

	t.Run("returns Unauthorized when the token's tenant has been deactivated", func(t *testing.T) {
		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "mytoken").
			Return(true, nil).
			Once().
			On("GetTenantIDFromToken", req.Context(), "mytoken").
			Return("deactivated-tenant-id", nil).
			Once()
		tenantManagerMock.
			On("GetTenantByID", req.Context(), "deactivated-tenant-id").
			Return(nil, tenant.ErrTenantDoesNotExist).
			Once()

		http.HandlerFunc(handler.PostRefreshToken).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		assert.JSONEq(t, `{"error": "Not authorized."}`, string(respBody))
	})

	t.Run("returns the refreshed token", func(t *testing.T) {
		ctx = sdpcontext.SetTokenInContext(ctx, "mytoken")

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		require.NoError(t, err)

		jwtManagerMock.
			On("ValidateToken", req.Context(), "mytoken").
			Return(true, nil).
			Times(2).
			On("GetTenantIDFromToken", req.Context(), "mytoken").
			Return("tenant-id", nil).
			Once().
			On("RefreshToken", req.Context(), "mytoken", mock.AnythingOfType("time.Time")).
			Return("myrefreshedtoken", nil).
			Once()
		tenantManagerMock.
			On("GetTenantByID", req.Context(), "tenant-id").
			Return(&schema.Tenant{ID: "tenant-id", Name: "tenant-name"}, nil).
			Once()

		http.HandlerFunc(handler.PostRefreshToken).ServeHTTP(w, req)

		resp := w.Result()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"token": "myrefreshedtoken"}`, string(respBody))
	})

	jwtManagerMock.AssertExpectations(t)
	tenantManagerMock.AssertExpectations(t)
}
