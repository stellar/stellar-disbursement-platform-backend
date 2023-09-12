package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/monitor"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type ContextKey string

const TokenContextKey ContextKey = "auth_token"

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
				log.Errorf("Error trying to monitor request time: %s", err)
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
			isValid, err := authManager.ValidateToken(ctx, token)
			if err != nil {
				err = fmt.Errorf("error validating auth token: %w", err)
				log.Ctx(ctx).Error(err)
				httperror.Unauthorized("", err, nil).Render(rw)
				return
			}

			if !isValid {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// Add the token to the request context
			ctx = context.WithValue(ctx, TokenContextKey, token)
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

			// policyStr := "default-src 'self'; script-src 'self'; frame-ancestors 'self'; form-action 'self';"
			rw.Header().Set("Content-Security-Policy", cspStr)
			next.ServeHTTP(rw, req)
		})
	}
}
