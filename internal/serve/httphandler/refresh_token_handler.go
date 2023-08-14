package httphandler

import (
	"errors"
	"net/http"

	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/middleware"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-auth/pkg/auth"
)

type RefreshTokenHandler struct {
	AuthManager auth.AuthManager
}

func (h RefreshTokenHandler) PostRefreshToken(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	token, ok := ctx.Value(middleware.TokenContextKey).(string)
	if !ok {
		httperror.Unauthorized("", nil, nil).Render(rw)
		return
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
