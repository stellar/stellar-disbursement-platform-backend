package httphandler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

type DeleteContactInfoHandler struct {
	NetworkPassphrase string
	Models            *data.Models
}

func (d DeleteContactInfoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if d.NetworkPassphrase != network.TestNetworkPassphrase {
		httperror.NotFound("", nil, nil).Render(w)
		return
	}

	contactInfo := strings.TrimSpace(chi.URLParam(r, "contact_info"))
	extras := map[string]interface{}{}
	if contactInfo == "" {
		extras["contact_info"] = "contact_info is required"
	} else {
		phoneNumberErr := utils.ValidatePhoneNumber(contactInfo)
		emailErr := utils.ValidateEmail(contactInfo)

		if phoneNumberErr != nil && emailErr != nil {
			extras["contact_info"] = "not a valid phone number or email"
		}
	}
	if len(extras) > 0 {
		httperror.BadRequest("", nil, extras).Render(w)
		return
	}

	log.Ctx(ctx).Warnf("Deleting user with phone number %s", utils.TruncateString(contactInfo, 3))
	err := d.Models.Receiver.DeleteByContactInfo(ctx, d.Models.DBConnectionPool, contactInfo)
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
