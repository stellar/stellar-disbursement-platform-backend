package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
	"github.com/stellar/go/support/http/mutil"
	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type ContextKey string

const (
	TokenContextKey ContextKey = "auth_token"
	TenantHeaderKey string     = "SDP-Tenant-Name"
)

// RecoverHandler is a middleware that recovers from panics and logs the error.
func RecoverHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("panic: %v", r)
			}

			// No need to recover when the client has disconnected:
			if errors.Is(err, http.ErrAbortHandler) {
				panic(err)
			}

			ctx := req.Context()
			log.Ctx(ctx).WithStack(err).Error(err)
			httperror.InternalError(ctx, "", err, nil).Render(rw)
		}()

		next.ServeHTTP(rw, req)
	})
}

// MetricsRequestHandler is a middleware that monitors http requests, and export the data
// to the metrics server
func MetricsRequestHandler(monitorService monitor.MonitorServiceInterface) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			mw := middleware.NewWrapResponseWriter(rw, req.ProtoMajor)
			then := time.Now()
			next.ServeHTTP(mw, req)

			duration := time.Since(then)

			labels := monitor.HttpRequestLabels{
				Status: fmt.Sprintf("%d", mw.Status()),
				Route:  utils.GetRoutePattern(req),
				Method: req.Method,
			}

			err := monitorService.MonitorHttpRequestDuration(duration, labels)
			if err != nil {
				log.Ctx(req.Context()).Errorf("Error trying to monitor request time: %s", err)
			}
		})
	}
}

// AuthenticateMiddleware is a middleware that validates the Authorization header for
// authenticated endpoints.
func AuthenticateMiddleware(authManager auth.AuthManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			authHeader := req.Header.Get("Authorization")
			if authHeader == "" {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			authHeaderParts := strings.Split(authHeader, " ")
			if len(authHeaderParts) != 2 {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			ctx := req.Context()
			token := authHeaderParts[1]
			userID, err := authManager.GetUserID(ctx, token)
			if err != nil {
				if !errors.Is(err, auth.ErrInvalidToken) && !errors.Is(err, auth.ErrUserNotFound) {
					err = fmt.Errorf("error validating auth token: %w", err)
					log.Ctx(ctx).Error(err)
				}
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// Add the token to the request context
			ctx = context.WithValue(ctx, TokenContextKey, token)

			// Add the user ID to the request context logger
			ctx = log.Set(ctx, log.Ctx(ctx).WithField("user_id", userID))

			req = req.WithContext(ctx)

			next.ServeHTTP(rw, req)
		})
	}
}

// AnyRoleMiddleware validates if the user has at least one of the required roles to request
// the current endpoint.
func AnyRoleMiddleware(authManager auth.AuthManager, requiredRoles ...data.UserRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			token, ok := ctx.Value(TokenContextKey).(string)
			if !ok {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// Accessible by all users
			if len(requiredRoles) == 0 {
				next.ServeHTTP(rw, req)
				return
			}

			isValid, err := authManager.AnyRolesInTokenUser(ctx, token, data.FromUserRoleArrayToStringArray(requiredRoles))
			if err != nil && !errors.Is(err, auth.ErrInvalidToken) && !errors.Is(err, auth.ErrUserNotFound) {
				httperror.InternalError(ctx, "", err, nil).Render(rw)
				return
			}

			if !isValid {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			next.ServeHTTP(rw, req)
		})
	}
}

func CorsMiddleware(corsAllowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		cors := cors.New(cors.Options{
			AllowedOrigins: corsAllowedOrigins,
			AllowedHeaders: []string{"*"},
			AllowedMethods: []string{"GET", "PUT", "POST", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		})

		return cors.Handler(next)
	}
}

type cspItem struct {
	ContentType string
	Policy      []string
}

func (c cspItem) String() string {
	return fmt.Sprintf("%s %s;", c.ContentType, strings.Join(c.Policy, " "))
}

// LoggingMiddleware is a middleware that logs requests to the logger.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		mw := mutil.WrapWriter(rw)

		reqCtx := req.Context()
		logFields := log.F{
			"method": req.Method,
			"path":   req.URL.String(),
			"req":    middleware.GetReqID(reqCtx),
		}
		logCtx := log.Set(reqCtx, log.Ctx(reqCtx).WithFields(logFields))

		ctxTenant, err := tenant.GetTenantFromContext(reqCtx)
		if err != nil {
			// Log for auditing purposes when we cannot derive the tenant from the context in the case of
			// tenant-unaware endpoints
			log.Ctx(logCtx).Debug("tenant cannot be derived from context")
		}
		if ctxTenant != nil {
			logFields["tenant_name"] = ctxTenant.Name
			logFields["tenant_id"] = ctxTenant.ID
			logCtx = log.Set(reqCtx, log.Ctx(reqCtx).WithFields(logFields))
		}

		req = req.WithContext(logCtx)

		logRequestStart(req)
		started := time.Now()

		next.ServeHTTP(mw, req)
		ended := time.Since(started)
		logRequestEnd(req, mw, ended)
	})
}

func logRequestStart(req *http.Request) {
	l := log.Ctx(req.Context()).WithFields(
		log.F{
			"subsys":    "http",
			"ip":        req.RemoteAddr,
			"host":      req.Host,
			"useragent": req.Header.Get("User-Agent"),
		},
	)

	l.Info("starting request")
}

func logRequestEnd(req *http.Request, mw mutil.WriterProxy, duration time.Duration) {
	l := log.Ctx(req.Context()).WithFields(log.F{
		"subsys":   "http",
		"status":   mw.Status(),
		"bytes":    mw.BytesWritten(),
		"duration": duration,
	})
	if routeContext := chi.RouteContext(req.Context()); routeContext != nil {
		l = l.WithField("route", routeContext.RoutePattern())
	}

	l.Info("finished request")
}

// CSPMiddleware is the middleware that sets the content security policy, restricting content to only be accessed
// from specified sources in the header.
func CSPMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			selfSrc := "'self'"
			recaptchaSrc := "https://www.google.com/recaptcha/"
			ipapiSrc := "https://ipapi.co/json"
			cspItems := []cspItem{
				{"script-src", []string{selfSrc, recaptchaSrc, "https://www.gstatic.com/recaptcha/"}},
				{"style-src", []string{selfSrc, recaptchaSrc, "https://fonts.googleapis.com/css2", "'unsafe-inline'"}},
				{"connect-src", []string{selfSrc, recaptchaSrc, ipapiSrc}},
				{"font-src", []string{selfSrc, "https://fonts.gstatic.com"}},
				{"default-src", []string{selfSrc}},

				{"frame-src", []string{selfSrc, recaptchaSrc}},
				{"frame-ancestors", []string{selfSrc}},

				{"form-action", []string{selfSrc}},
			}
			cspStr := ""
			for _, item := range cspItems {
				cspStr += fmt.Sprintf("%v", item)
			}

			rw.Header().Set("Content-Security-Policy", cspStr)
			next.ServeHTTP(rw, req)
		})
	}
}

// InjectTenantMiddleware is a middleware that injects the tenant into the request context, if it can be found in either
// the authentication token, the request HEADER, or the hostname prefix.
func InjectTenantMiddleware(tenantManager tenant.ManagerInterface, authManager auth.AuthManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			var currentTenant *tenant.Tenant
			// Attempt 1. Attempt fetching tenant ID from token
			if token, ok := ctx.Value(TokenContextKey).(string); ok {
				if tenantID, err := authManager.GetTenantID(ctx, token); err == nil {
					currentTenant, _ = tenantManager.GetTenantByID(ctx, tenantID)
				}
			}

			// Attempt 2. Attempt fetching tenant name from request
			if currentTenant == nil {
				if tenantName, err := extractTenantNameFromRequest(req); err == nil && tenantName != "" {
					currentTenant, _ = tenantManager.GetTenantByName(ctx, tenantName)
				}
			}

			if currentTenant != nil {
				ctx = tenant.SaveTenantInContext(ctx, currentTenant)
				next.ServeHTTP(rw, req.WithContext(ctx))
			} else {
				next.ServeHTTP(rw, req)
			}
		})
	}
}

// EnsureTenantMiddleware is a middleware that ensures the tenant is in the request context.
func EnsureTenantMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		if _, err := tenant.GetTenantFromContext(ctx); err != nil {
			httperror.BadRequest("Tenant not found in context", err, nil).Render(rw)
			return
		}

		next.ServeHTTP(rw, req)
	})
}

func BasicAuthMiddleware(adminAccount, adminApiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			if adminAccount == "" || adminApiKey == "" {
				httperror.InternalError(ctx, "Admin account and API key are not set", nil, nil).Render(rw)
				return
			}

			accountUserName, apiKey, ok := req.BasicAuth()
			if !ok {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// Using constant time comparison to avoid timing attacks
			if accountUserName != adminAccount || subtle.ConstantTimeCompare([]byte(apiKey), []byte(adminApiKey)) != 1 {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			log.Ctx(ctx).Infof("[AdminAuth] - Admin authenticated with account %s", adminAccount)
			next.ServeHTTP(rw, req)
		})
	}
}

// extractTenantNameFromRequest attempts to extract the tenant name from the request HEADER[tenantHeaderKey] or the hostname prefix.
func extractTenantNameFromRequest(r *http.Request) (string, error) {
	// 1. Try extracting from the TenantHeaderKey header first
	tenantName := r.Header.Get(TenantHeaderKey)
	if tenantName != "" {
		return tenantName, nil
	}

	// 2. If header is blank, extract from the hostname prefix
	return utils.ExtractTenantNameFromHostName(r.Host)
}
