package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type ctxKey string

const APIKeyContextKey ctxKey = "api_key"

// APIKeyOrJWTAuthenticate first checks for an SDP_ key, then falls back to JWT.
func APIKeyOrJWTAuthenticate(apiKeyModel *data.APIKeyModel, jwtAuth func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, data.APIKeyPrefix) {
				// 1) Validate and fetch APIKey
				apiKey, err := apiKeyModel.ValidateRawKey(r.Context(), auth)
				if err != nil {
					httperror.Unauthorized("Invalid API key", nil, nil).Render(w)
					return
				}
				// 2) Stamp last_used_at
				if err := apiKeyModel.UpdateLastUsed(r.Context(), apiKey.ID); err != nil {
					log.Ctx(r.Context()).Warnf("could not update last_used_at for API key %s: %s", apiKey.ID, err)
				}
				// 3) IP & expiry already checked in ValidateRawKey/IsAllowedIP
				if !apiKey.IsAllowedIP(strings.Split(r.RemoteAddr, ":")[0]) {
					httperror.Forbidden("IP not allowed", nil, nil).Render(w)
					return
				}

				ctx := context.WithValue(r.Context(), APIKeyContextKey, apiKey)
				ctx = context.WithValue(ctx, UserIDContextKey, apiKey.CreatedBy)
				ctx = log.Set(ctx, log.Ctx(ctx).WithField("user_id", apiKey.CreatedBy))

				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			jwtAuth(next).ServeHTTP(w, r)
		})
	}
}

// RequirePermission ensures the caller has the given APIKeyPermission or (if no APIKey in context) passes through to the next role-check.
func RequirePermission(
	perm data.APIKeyPermission,
	jwtCheck func(http.Handler) http.Handler,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// --- API key path ---
			if raw := r.Context().Value(APIKeyContextKey); raw != nil {
				a := raw.(*data.APIKey)
				if !a.HasPermission(perm) {
					httperror.Forbidden("Insufficient API key permissions", nil, nil).Render(w)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			// --- JWT path: delegate ---
			jwtCheck(next).ServeHTTP(w, r)
		})
	}
}
