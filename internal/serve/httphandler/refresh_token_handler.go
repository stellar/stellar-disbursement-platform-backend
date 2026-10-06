package httphandler

import (
	"errors"
	"net/http"

	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type RefreshTokenHandler struct {
	AuthManager   auth.AuthManager
	TenantManager tenant.ManagerInterface
}

func (h RefreshTokenHandler) PostRefreshToken(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token, err := sdpcontext.GetTokenFromContext(ctx)
	if err != nil {
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
	}

	// Don't extend session when token's own tenant no longer resolves (deactivated / deleted)
	if tenantID, tenantIDErr := h.AuthManager.GetTenantID(ctx, token); tenantIDErr == nil && tenantID != "" {
		if _, tenantErr := h.TenantManager.GetTenantByID(ctx, tenantID); tenantErr != nil {
			if errors.Is(tenantErr, tenant.ErrTenantDoesNotExist) {
				httperror.Unauthorized("", tenantErr, nil).Render(rw)
				return
			}
			httperror.InternalError(ctx, "Cannot resolve tenant for token refresh", tenantErr, nil).Render(rw)
			return
		}
	}

	refreshedToken, err := h.AuthManager.RefreshToken(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			httperror.BadRequest("", err, map[string]interface{}{"token": "token is invalid"}).Render(rw)
			return
		}

		httperror.InternalError(ctx, "Cannot refresh user token", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, map[string]string{"token": refreshedToken}, httpjson.JSON)
}
