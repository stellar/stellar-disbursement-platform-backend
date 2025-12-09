package sepauth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stellar/go-stellar-sdk/strkey"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

// WebAuthClaimsContextKey is the context key used to store web auth claims.
const WebAuthClaimsContextKey ContextType = "webauth_claims"

// WebAuthClaims is a wrapper around SEP-10 or SEP-45 JWT claims used for web authentication.
type WebAuthClaims struct {
	Subject      string
	ClientDomain string
	HomeDomain   string
	TokenType    WebAuthTokenType
}

type WebAuthTokenType string

const (
	// WebAuthTokenTypeSEP10 indicates a web auth token derived from SEP-10.
	WebAuthTokenTypeSEP10 WebAuthTokenType = "sep10"
	// WebAuthTokenTypeSEP45 indicates a web auth token derived from SEP-45.
	WebAuthTokenTypeSEP45 WebAuthTokenType = "sep45"
)

// GetWebAuthClaims retrieves web auth claims from the request context, if present.
func GetWebAuthClaims(ctx context.Context) *WebAuthClaims {
	claims := ctx.Value(WebAuthClaimsContextKey)
	if claims == nil {
		return nil
	}
	return claims.(*WebAuthClaims)
}

// WebAuthHeaderTokenAuthenticateMiddleware validates a JWT provided via the Authorization
// header (Authorization: Bearer <token>). It accepts either a SEP-10 or SEP-45 token and stores
// the parsed claims in the request context.
func WebAuthHeaderTokenAuthenticateMiddleware(jwtManager *JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			authHeader := req.Header.Get("Authorization")
			if authHeader == "" {
				httperror.Unauthorized("Missing authorization header", nil, nil).Render(rw)
				return
			}

			if !strings.HasPrefix(authHeader, "Bearer ") {
				httperror.BadRequest("Invalid authorization header", nil, nil).Render(rw)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			if strings.TrimSpace(token) == "" {
				httperror.Unauthorized("Invalid token", nil, nil).Render(rw)
				return
			}

			var (
				webAuthClaims *WebAuthClaims
				sep45Claims   *Sep45JWTClaims
				sep10Claims   *Sep10JWTClaims
			)

			if claims, err := jwtManager.ParseSEP45TokenClaims(token); err == nil {
				sep45Claims = claims
			} else {
				var validationErr *jwt.ValidationError
				if errors.As(err, &validationErr) && validationErr.Is(jwt.ErrTokenExpired) {
					httperror.Unauthorized("Expired token", nil, nil).Render(rw)
					return
				}
			}

			if claims, err := jwtManager.ParseSEP10TokenClaims(token); err == nil {
				sep10Claims = claims
			} else {
				var validationErr *jwt.ValidationError
				if errors.As(err, &validationErr) && validationErr.Is(jwt.ErrTokenExpired) {
					httperror.Unauthorized("Expired token", nil, nil).Render(rw)
					return
				}
			}

			switch {
			case sep45Claims != nil && strkey.IsValidContractAddress(sep45Claims.Subject):
				webAuthClaims = &WebAuthClaims{
					Subject:      sep45Claims.Subject,
					ClientDomain: sep45Claims.ClientDomain,
					HomeDomain:   sep45Claims.HomeDomain,
					TokenType:    WebAuthTokenTypeSEP45,
				}
			case sep10Claims != nil:
				webAuthClaims = &WebAuthClaims{
					Subject:      sep10Claims.Subject,
					ClientDomain: sep10Claims.ClientDomain,
					HomeDomain:   sep10Claims.HomeDomain,
					TokenType:    WebAuthTokenTypeSEP10,
				}
			default:
				httperror.Unauthorized("Invalid token", nil, nil).Render(rw)
				return
			}

			ctx = context.WithValue(ctx, WebAuthClaimsContextKey, webAuthClaims)
			req = req.WithContext(ctx)

			next.ServeHTTP(rw, req)
		})
	}
}
