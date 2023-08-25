package anchorplatform

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/go/network"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type ContextType string

const SEP24ClaimsContextKey ContextType = "sep24_claims"

func GetSEP24Claims(ctx context.Context) *SEP24JWTClaims {
	claims := ctx.Value(SEP24ClaimsContextKey)
	if claims == nil {
		return nil
	}
	return claims.(*SEP24JWTClaims)
}

type SEP24RequestQuery struct {
	Token         string `query:"token"`
	TransactionID string `query:"transaction_id"`
}

// checkSEP24ClientDomain check if the sep24 token has a client domain and if not check in which network the API is running on,
// only testnet can have an empty client domain.
func checkSEP24ClientDomain(ctx context.Context, sep24Claims *SEP24JWTClaims, networkPassphrase string) error {
	if sep24Claims.ClientDomain() == "" {
		missingDomain := "missing client domain in the token claims"
		if networkPassphrase == network.PublicNetworkPassphrase {
			log.Ctx(ctx).Error(missingDomain)
			return fmt.Errorf(missingDomain)
		}
		log.Ctx(ctx).Warn(missingDomain)
	}
	return nil
}

// SEP24QueryTokenAuthenticateMiddleware is a middleware that validates if the token passed in as a query
// parameter with ?token={token} is valid for the authenticated endpoints.
func SEP24QueryTokenAuthenticateMiddleware(jwtManager *JWTManager, networkPassphrase string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			// get the token from the request query parameters
			var reqParams SEP24RequestQuery
			if err := httpdecode.DecodeQuery(req, &reqParams); err != nil {
				err = fmt.Errorf("decoding the request query parameters: %w", err)
				log.Ctx(ctx).Error(err)
				httperror.BadRequest("", err, nil).Render(rw)
				return
			}

			// check if the token is present
			if reqParams.Token == "" {
				log.Ctx(ctx).Error("no token was provided in the request")
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// parse the token claims
			sep24Claims, err := jwtManager.ParseSEP24TokenClaims(reqParams.Token)
			if err != nil {
				err = fmt.Errorf("parsing the token claims: %w", err)
				log.Ctx(ctx).Error(err)
				httperror.Unauthorized("", err, nil).Render(rw)
				return
			}

			// check if the transaction_id in the token claims matches the transaction_id in the request query parameters
			if sep24Claims.TransactionID() != reqParams.TransactionID {
				log.Ctx(ctx).Error("the transaction_id in the token claims does not match the transaction_id in the request query parameters")
				httperror.BadRequest("", nil, nil).Render(rw)
				return
			}

			err = checkSEP24ClientDomain(ctx, sep24Claims, networkPassphrase)
			if err != nil {
				httperror.BadRequest("", err, nil).Render(rw)
				return
			}

			// Add the token to the request context
			ctx = context.WithValue(ctx, SEP24ClaimsContextKey, sep24Claims)
			req = req.WithContext(ctx)

			next.ServeHTTP(rw, req)
		})
	}
}

// SEP24HeaderTokenAuthenticateMiddleware is a middleware that validates if the token passed in
// the 'Authorization' header is valid for the authenticated endpoints.
func SEP24HeaderTokenAuthenticateMiddleware(jwtManager *JWTManager, networkPassphrase string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			ctx := req.Context()

			// get the token from the Authorization header
			authHeader := req.Header.Get("Authorization")
			// check if the Authorization header is present
			if authHeader == "" {
				log.Ctx(ctx).Error("no token was provided in the Authorization header")
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// check if the Authorization header has two parts ['Bearer', token]
			if !strings.HasPrefix(authHeader, "Bearer ") {
				log.Ctx(ctx).Error("invalid Authorization header provided")
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			// parse the token claims
			token := strings.Replace(authHeader, "Bearer ", "", 1)
			sep24Claims, err := jwtManager.ParseSEP24TokenClaims(token)
			if err != nil {
				err = fmt.Errorf("parsing the token claims: %w", err)
				log.Ctx(ctx).Error(err)

				httperror.Unauthorized("", err, nil).Render(rw)
				return
			}

			err = checkSEP24ClientDomain(ctx, sep24Claims, networkPassphrase)
			if err != nil {
				httperror.BadRequest("", err, nil).Render(rw)
				return
			}

			// Add the token to the request context
			ctx = context.WithValue(ctx, SEP24ClaimsContextKey, sep24Claims)
			req = req.WithContext(ctx)

			next.ServeHTTP(rw, req)
		})
	}
}
