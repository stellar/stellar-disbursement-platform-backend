package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type ctxKey string

const (
	APIKeyContextKey ctxKey = "api_key"
	apiKeyCacheTTL          = 3 * time.Minute
)

type apiKeyAuthenticator struct {
	model *data.APIKeyModel
	cache *ristretto.Cache
}

func newAPIKeyAuthenticator(model *data.APIKeyModel) *apiKeyAuthenticator {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10_000,
		MaxCost:     1_000,
		BufferItems: 64,
	})
	if err != nil {
		log.Errorf("Failed to create API key cache: %v", err)
		return &apiKeyAuthenticator{model: model}
	}

	cache.Wait()

	return &apiKeyAuthenticator{
		model: model,
		cache: cache,
	}
}

func (a *apiKeyAuthenticator) validate(ctx context.Context, rawKey string) (*data.APIKey, error) {
	if a.cache == nil {
		return a.model.ValidateRawKeyAndUpdateLastUsed(ctx, rawKey)
	}

	if cached, found := a.cache.Get(rawKey); found {
		if apiKey, ok := cached.(*data.APIKey); ok && !apiKey.IsExpired() {
			return apiKey, nil
		}
		a.cache.Del(rawKey)
	}

	apiKey, err := a.model.ValidateRawKeyAndUpdateLastUsed(ctx, rawKey)
	if err != nil {
		return nil, err
	}

	if !apiKey.IsExpired() {
		a.cache.SetWithTTL(rawKey, apiKey, 1, apiKeyCacheTTL)
	}

	return apiKey, nil
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if parts := strings.Split(auth, " "); len(parts) == 2 {
		return parts[1]
	}
	return auth
}

func extractClientIP(r *http.Request) string {
	// Check proxy headers first
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if ip := r.Header.Get(header); ip != "" {
			if comma := strings.Index(ip, ","); comma > 0 {
				ip = ip[:comma]
			}
			return strings.TrimSpace(ip)
		}
	}

	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if host == "" {
		return r.RemoteAddr
	}
	return host
}

func setAPIKeyContext(r *http.Request, apiKey *data.APIKey) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, APIKeyContextKey, apiKey)
	ctx = context.WithValue(ctx, UserIDContextKey, apiKey.CreatedBy)
	ctx = log.Set(ctx, log.Ctx(ctx).WithField("user_id", apiKey.CreatedBy))
	return r.WithContext(ctx)
}

// APIKeyOrJWTAuthenticate first checks for an SDP_ key, then falls back to JWT.
func APIKeyOrJWTAuthenticate(apiKeyModel *data.APIKeyModel, jwtAuth func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	auth := newAPIKeyAuthenticator(apiKeyModel)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)

			if !strings.HasPrefix(token, data.APIKeyPrefix) {
				jwtAuth(next).ServeHTTP(w, r)
				return
			}

			apiKey, err := auth.validate(r.Context(), token)
			if err != nil {
				httperror.Unauthorized("Invalid API key", nil, nil).Render(w)
				return
			}

			if clientIP := extractClientIP(r); !apiKey.IsAllowedIP(clientIP) {
				httperror.Forbidden("IP not allowed", nil, nil).Render(w)
				return
			}

			next.ServeHTTP(w, setAPIKeyContext(r, apiKey))
		})
	}
}

// RequirePermission ensures the caller has the given APIKeyPermission or (if no APIKey in context) passes through to the next role-check.
func RequirePermission(perm data.APIKeyPermission, jwtCheck func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// --- API key path ---
			if apiKey, ok := r.Context().Value(APIKeyContextKey).(*data.APIKey); ok {
				if !apiKey.HasPermission(perm) {
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
