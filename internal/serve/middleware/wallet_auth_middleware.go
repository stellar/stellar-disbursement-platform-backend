package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/go/support/log"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/wallet"
)

func WalletAuthMiddleware(walletJWTManager wallet.WalletJWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			authHeader := req.Header.Get("Authorization")
			if authHeader == "" {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			authHeaderParts := strings.Split(authHeader, " ")
			if len(authHeaderParts) != 2 || authHeaderParts[0] != "Bearer" {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			ctx := req.Context()
			token := authHeaderParts[1]

			contractAddress, err := walletJWTManager.ValidateToken(ctx, token)
			if err != nil {
				if !errors.Is(err, wallet.ErrInvalidWalletToken) &&
					!errors.Is(err, wallet.ErrExpiredWalletToken) &&
					!errors.Is(err, wallet.ErrMissingSubClaim) {
					err = fmt.Errorf("error validating wallet token: %w", err)
					log.Ctx(ctx).Error(err)
				}
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			if contractAddress == "" {
				httperror.Unauthorized("", nil, nil).Render(rw)
				return
			}

			ctx = sdpcontext.SetWalletContractAddressInContext(ctx, contractAddress)
			ctx = sdpcontext.SetTokenInContext(ctx, token)
			ctx = log.Set(ctx, log.Ctx(ctx).WithField("wallet_contract_address", contractAddress))

			req = req.WithContext(ctx)

			next.ServeHTTP(rw, req)
		})
	}
}
