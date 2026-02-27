package middleware

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
	"github.com/stellar/go-stellar-sdk/support/http/mutil"
	"github.com/stellar/go-stellar-sdk/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/pkg/schema"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

const (
	TenantHeaderKey string = "SDP-Tenant-Name"
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
			httperror.InternalError(ctx, "", err, nil).WithErrorCode(httperror.Code500_0).Render(rw)
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

			labels := monitor.HTTPRequestLabels{
				Status: fmt.Sprintf("%d", mw.Status()),
				Route:  utils.GetRoutePattern(req),
				Method: req.Method,
				CommonLabels: monitor.CommonLabels{
					TenantName: sdpcontext.MustGetTenantNameFromContext(req.Context()),
				},
			}

			if err := monitorService.MonitorHTTPRequestDuration(duration, labels); err != nil {
				log.Ctx(req.Context()).Errorf("Error trying to monitor request time: %s", err)
			}
		})
	}
}

// AuthenticateMiddleware is a middleware that validates the Authorization header for
// authenticated endpoints.
func AuthenticateMiddleware(authManager auth.AuthManager, tenantManager tenant.ManagerInterface) func(http.Handler) http.Handler {
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
			ctx = sdpcontext.SetTokenInContext(ctx, token)
			ctx = sdpcontext.SetUserIDInContext(ctx, userID)

			// Attempt fetching tenant ID from token
			tenantID, err := authManager.GetTenantID(ctx, token)
			if err == nil && tenantID != "" {
				currentTenant, tenantErr := tenantManager.GetTenantByID(ctx, tenantID)
				if tenantErr == nil && currentTenant != nil {
					ctx = sdpcontext.SetTenantInContext(ctx, currentTenant)
				}
			}

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

			token, err := sdpcontext.GetTokenFromContext(ctx)
			if err != nil {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// Accessible by all users
			if len(requiredRoles) == 0 {
				next.ServeHTTP(rw, req)
				return
			}

			hasAnyRoles, err := authManager.AnyRolesInTokenUser(ctx, token, data.FromUserRoleArrayToStringArray(requiredRoles))
			if err != nil {
				if errors.Is(err, auth.ErrInvalidToken) || errors.Is(err, auth.ErrUserNotFound) {
					httperror.Unauthorized("", nil, nil).Render(rw)
				} else {
					httperror.InternalError(ctx, "", err, nil).Render(rw)
				}
				return
			}

			if !hasAnyRoles {
				httperror.Forbidden("", nil, nil).Render(rw)
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

		ctxTenant, err := sdpcontext.GetTenantFromContext(reqCtx)
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

// ResolveTenantFromRequestMiddleware is a middleware that injects the tenant into the request context, if it can be found in
// the request HEADER, or the hostname prefix.
func ResolveTenantFromRequestMiddleware(tenantManager tenant.ManagerInterface, singleTenantMode bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			var currentTenant *schema.Tenant
			if singleTenantMode {
				var err error
				currentTenant, err = tenantManager.GetDefault(ctx)
				if err != nil {
					switch {
					case errors.Is(err, tenant.ErrTenantDoesNotExist):
						// Log warning and allow the request to continue without a tenant.
						log.Ctx(ctx).Warnf(
							"No default tenant configured: %v. "+
								"use POST /default-tenant to set the default tenant.",
							err,
						)
						next.ServeHTTP(rw, req)
						return
					case errors.Is(err, tenant.ErrTooManyDefaultTenants):
						httperror.InternalError(ctx, "Too many default tenants configured", err, nil).Render(rw)
					default:
						httperror.InternalError(ctx, "", fmt.Errorf("error getting default tenant: %w", err), nil).Render(rw)
					}
					return
				}
			} else {
				// Attempt fetching tenant name from request
				tenantName, err := extractTenantNameFromRequest(req)
				if err != nil {
					if errors.Is(err, utils.ErrHostnameIsIPAddress) {
						log.Ctx(ctx).Debug("hostname is an IP address, skipping tenant resolution")
					} else if !errors.Is(err, utils.ErrTenantNameNotFound) {
						log.Ctx(ctx).Debugf("could not extract tenant name from request: %v", err)
					}
				} else if tenantName != "" {
					currentTenant, err = tenantManager.GetTenantByName(ctx, tenantName)
					if err != nil {
						log.Ctx(ctx).Warnf("could not find tenant with name %s: %v", tenantName, err)
					}
				}
			}

			if currentTenant != nil {
				ctx = sdpcontext.SetTenantInContext(ctx, currentTenant)
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

		if _, err := sdpcontext.GetTenantFromContext(ctx); err != nil {
			httperror.BadRequest("Tenant not found in context", err, nil).Render(rw)
			return
		}

		next.ServeHTTP(rw, req)
	})
}

func BasicAuthMiddleware(adminAccount, adminAPIKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			if adminAccount == "" || adminAPIKey == "" {
				httperror.InternalError(ctx, "Admin account and API key are not set", nil, nil).Render(rw)
				return
			}

			accountUserName, apiKey, ok := req.BasicAuth()
			if !ok {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// Using constant time comparison to avoid timing attacks
			if accountUserName != adminAccount || subtle.ConstantTimeCompare([]byte(apiKey), []byte(adminAPIKey)) != 1 {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			log.Ctx(ctx).Infof("[AdminAuth] - Admin authenticated with account %s", adminAccount)
			next.ServeHTTP(rw, req)
		})
	}
}

// DefaultMaxRequestBodySize is the default maximum request body size (10 MB) applied globally (CWE-770).
const DefaultMaxRequestBodySize int64 = 10 * 1024 * 1024

// MaxBodySize is a middleware that limits the size of the request body using http.MaxBytesReader (CWE-770).
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			req.Body = http.MaxBytesReader(rw, req.Body, maxBytes)
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
	tenantName, err := utils.ExtractTenantNameFromHostName(r.Host)
	if err != nil {
		return "", fmt.Errorf("extracting tenant name from hostname: %w", err)
	}
	return tenantName, nil
}
