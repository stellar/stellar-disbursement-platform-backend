package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

// apiKeyCacheTTL bounds how long a validated API key can be served from cache
// without a fresh DB check. It exists only as defense-in-depth (e.g. across
// multiple server instances, each with their own in-process cache); within a
// single instance, Invalidate is called synchronously by the API key
// handler on every permission/IP-allowlist change and on delete, so a given
// instance never needs to wait out the TTL to observe an update. Keep this
// short: it is a security control (permissions, IP allowlist), not a
// read-through cache for inert data, so a stale hit has real consequences.
const apiKeyCacheTTL = 10 * time.Second

// APIKeyAuthenticator authenticates SDP_-prefixed bearer tokens and caches
// validated keys - keyed by "<tenant ID>:<API key ID>", never by the raw
// secret and never by ID alone - so most requests avoid a DB round trip.
// Tenant-scoping the cache key prevents a raw key that is only valid for one
// tenant's schema from being served out of a cache entry warmed by a
// different tenant's context: the request's tenant comes from the
// attacker-controlled SDP-Tenant-Name header, so without this scoping a
// replayed key could ride another tenant's cached validation and bypass the
// schema-scoped DB lookup that is the sole mechanism binding a key to its
// tenant. Callers that mutate an API key's permissions, allowed IPs, or
// existence (update/delete) MUST call Invalidate(ctx, id) as part of that
// operation, or the cache can keep authorizing requests against the stale,
// pre-change permission set.
type APIKeyAuthenticator struct {
	model *data.APIKeyModel
	cache *ristretto.Cache
}

// NewAPIKeyAuthenticator builds an APIKeyAuthenticator backed by model.
func NewAPIKeyAuthenticator(model *data.APIKeyModel) *APIKeyAuthenticator {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 10_000,
		MaxCost:     1_000,
		BufferItems: 64,
	})
	if err != nil {
		log.Errorf("Failed to create API key cache: %v", err)
		return &APIKeyAuthenticator{model: model}
	}

	cache.Wait()

	return &APIKeyAuthenticator{
		model: model,
		cache: cache,
	}
}

// cacheKeyFor builds the tenant-scoped cache key for the API key with the
// given ID, using the tenant resolved from ctx. ok is false (and key empty)
// when the tenant can't be resolved, in which case the cache must not be
// consulted at all - see validate for why.
func cacheKeyFor(ctx context.Context, id string) (key string, ok bool) {
	tenant, err := sdpcontext.GetTenantFromContext(ctx)
	if err != nil || tenant == nil || tenant.ID == "" {
		return "", false
	}
	return tenant.ID + ":" + id, true
}

// Invalidate evicts any cached validation result for the API key with the
// given ID, scoped to the tenant resolved from ctx (the same tenant the
// request that is updating/deleting the key is itself running under). It
// must be called whenever that key's permissions or allowed_ips are updated,
// or when the key is deleted, so the very next request using it is forced to
// re-fetch current authorization data from the DB instead of serving a stale
// cache hit.
func (a *APIKeyAuthenticator) Invalidate(ctx context.Context, id string) {
	if a == nil || a.cache == nil {
		return
	}
	if cacheKey, ok := cacheKeyFor(ctx, id); ok {
		a.cache.Del(cacheKey)
	}
}

func (a *APIKeyAuthenticator) validate(ctx context.Context, rawKey string) (*data.APIKey, error) {
	// The cache is keyed by "<tenant ID>:<API key ID>" rather than the raw
	// secret:
	//  - Tenant-scoping closes a cross-tenant cache-poisoning hole (see the
	//    type doc above): without it, a key cached while serving tenant A
	//    could be replayed under tenant B's name and served straight out of
	//    cache, bypassing the schema-scoped DB lookup that binds a key to
	//    its tenant.
	//  - Keying on the ID (not the secret) is what lets the /api-keys/{id}
	//    update/delete handlers evict exactly this entry: the raw secret is
	//    never persisted after creation, so those handlers have no way to
	//    derive it, only the ID.
	// The secret is still verified against the cached hash on every request
	// (cache hit or not), so a cache hit never skips authentication - it
	// only skips the DB round trip.
	id, secret, parseErr := data.ParseRawAPIKey(rawKey)

	var cacheKey string
	cacheable := false
	if parseErr == nil {
		cacheKey, cacheable = cacheKeyFor(ctx, id)
	}

	if a.cache == nil || !cacheable {
		apiKey, err := a.model.ValidateRawKeyAndUpdateLastUsed(ctx, rawKey)
		if err != nil {
			return nil, fmt.Errorf("validating API key (cacheless): %w", err)
		}
		return apiKey, nil
	}

	if cached, found := a.cache.Get(cacheKey); found {
		if apiKey, ok := cached.(*data.APIKey); ok &&
			!apiKey.IsExpired() &&
			apiKey.VerifySecret(secret) {
			return apiKey, nil
		}
		a.cache.Del(cacheKey)
	}

	apiKey, err := a.model.ValidateRawKeyAndUpdateLastUsed(ctx, rawKey)
	if err != nil {
		return nil, fmt.Errorf("validating API key: %w", err)
	}

	if !apiKey.IsExpired() {
		a.cache.SetWithTTL(cacheKey, apiKey, 1, apiKeyCacheTTL)
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

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		return r.RemoteAddr
	}
	return host
}

// Middleware first checks for an SDP_ key, then falls back to JWT.
func (a *APIKeyAuthenticator) Middleware(jwtAuth func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)

			if !strings.HasPrefix(token, data.APIKeyPrefix) {
				jwtAuth(next).ServeHTTP(w, r)
				return
			}

			apiKey, err := a.validate(r.Context(), token)
			if err != nil {
				httperror.Unauthorized("Invalid API key", nil, nil).Render(w)
				return
			}

			if clientIP := extractClientIP(r); !apiKey.IsAllowedIP(clientIP) {
				httperror.Forbidden("IP not allowed", nil, nil).Render(w)
				return
			}

			// Set API key context
			ctx := r.Context()
			ctx = sdpcontext.SetAPIKeyInContext(ctx, apiKey)
			ctx = sdpcontext.SetUserIDInContext(ctx, apiKey.CreatedBy)
			ctx = log.Set(ctx, log.Ctx(ctx).WithField("user_id", apiKey.CreatedBy))

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyOrJWTAuthenticate builds a one-off APIKeyAuthenticator and returns its
// middleware. Prefer constructing an APIKeyAuthenticator once (via
// NewAPIKeyAuthenticator) and sharing it with whatever also needs to call
// Invalidate (e.g. the API key management handlers) - this helper is kept for
// callers (and tests) that don't need to invalidate the cache themselves.
func APIKeyOrJWTAuthenticate(apiKeyModel *data.APIKeyModel, jwtAuth func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return NewAPIKeyAuthenticator(apiKeyModel).Middleware(jwtAuth)
}

// RequirePermission ensures the caller has the given APIKeyPermission or (if no APIKey in context) passes through to the next role-check.
func RequirePermission(perm data.APIKeyPermission, jwtCheck func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// --- API key path ---
			if apiKey, err := sdpcontext.GetAPIKeyFromContext(r.Context()); err == nil {
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
