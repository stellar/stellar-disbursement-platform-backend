package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/db"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type ReceiverWalletsHandler struct {
	Models           *data.Models
	DBConnectionPool db.DBConnectionPool
}

func (h ReceiverWalletsHandler) RetryInvitation(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	receiverWalletID := chi.URLParam(req, "receiver_wallet_id")
	receiverWallet, err := h.Models.ReceiverWallet.RetryInvitationSMS(ctx, receiverWalletID, h.DBConnectionPool)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("", err, nil).Render(rw)
			return
		}
		err = fmt.Errorf("finding the receiver: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.InternalError(ctx, "", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusOK, receiverWallet, httpjson.JSON)
}
