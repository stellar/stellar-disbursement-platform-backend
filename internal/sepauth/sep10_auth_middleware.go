package sepauth

import (
	"context"
	"net/http"
	"strings"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

// SEP10ClaimsContextKey is the context key used to store SEP-10 claims in the request context.
const SEP10ClaimsContextKey ContextType = "sep10_claims"

// GetSEP10Claims retrieves SEP-10 claims from the request context, if present.
func GetSEP10Claims(ctx context.Context) *Sep10JWTClaims {
	claims := ctx.Value(SEP10ClaimsContextKey)
	if claims == nil {
		return nil
	}
	return claims.(*Sep10JWTClaims)
}

// SEP10HeaderTokenAuthenticateMiddleware validates a SEP-10 JWT provided via the Authorization header
// (Authorization: Bearer <token>). On success, it stores the parsed claims in the request context.
func SEP10HeaderTokenAuthenticateMiddleware(jwtManager *JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			authHeader := req.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				httperror.Forbidden("Missing or invalid authorization header", nil, nil).Render(rw)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			sep10Claims, err := jwtManager.ParseSEP10TokenClaims(token)
			if err != nil {
				httperror.Forbidden("Invalid token", err, nil).Render(rw)
				return
			}

			ctx = context.WithValue(ctx, SEP10ClaimsContextKey, sep10Claims)
			req = req.WithContext(ctx)

			next.ServeHTTP(rw, req)
		})
	}
}
