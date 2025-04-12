package middleware

import (
	"net/http"

	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

func TenantResolutionMiddleware(manager tenant.ManagerInterface, singleTenantMode bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			if singleTenantMode {
				defaultTenant, err := manager.EnsureDefaultTenant(ctx)
				if err == nil {
					ctx = tenant.SaveTenantInContext(ctx, defaultTenant)
					r = r.WithContext(ctx)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
