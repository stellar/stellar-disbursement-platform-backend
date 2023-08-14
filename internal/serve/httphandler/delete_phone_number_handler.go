package httphandler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/network"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type DeletePhoneNumberHandler struct {
	NetworkPassphrase string
	Models            *data.Models
}

func (d DeletePhoneNumberHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if d.NetworkPassphrase != network.TestNetworkPassphrase {
		httperror.NotFound("", nil, nil).Render(w)
		return
	}

	phoneNumber := chi.URLParam(r, "phone_number")
	if err := utils.ValidatePhoneNumber(phoneNumber); err != nil {
		extras := map[string]interface{}{"phone_number": "invalid phone number"}
		httperror.BadRequest("", nil, extras).Render(w)
		return
	}

	log.Ctx(ctx).Warnf("Deleting user with phone number %s", utils.TruncateString(phoneNumber, 3))
	err := d.Models.Receiver.DeleteByPhoneNumber(ctx, d.Models.DBConnectionPool, phoneNumber)
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			httperror.NotFound("", err, nil).Render(w)
		} else {
			httperror.InternalError(ctx, "Cannot delete phone number", err, nil).Render(w)
		}
		return
	}

	httpjson.RenderStatus(w, http.StatusNoContent, nil, httpjson.JSON)
}
