package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/go/support/log"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	htmlTpl "github.com/stellar/stellar-disbursement-platform-backend/internal/htmltemplate"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
)

type ReceiverRegistrationHandler struct {
	ReceiverWalletModel *data.ReceiverWalletModel
	ReCAPTCHASiteKey    string
}

type ReceiverRegistrationData struct {
	StellarAccount   string
	JWTToken         string
	Title            string
	Message          string
	ReCAPTCHASiteKey string
}

// ServeHTTP will serve the SEP-24 deposit page needed to register users.
func (h ReceiverRegistrationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sep24Claims := anchorplatform.GetSEP24Claims(ctx)
	if sep24Claims == nil {
		err := fmt.Errorf("no SEP-24 claims found in the request context")
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		err := fmt.Errorf("no token was provided in the request")
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	err := sep24Claims.Valid()
	if err != nil {
		err = fmt.Errorf("SEP-24 claims are invalid: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	rw, err := h.ReceiverWalletModel.GetByStellarAccountAndMemo(ctx, sep24Claims.SEP10StellarAccount(), sep24Claims.SEP10StellarMemo(), sep24Claims.ClientDomain())
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		httperror.InternalError(ctx, "Cannot register receiver wallet", err, nil).Render(w)
		return
	}

	tmplData := ReceiverRegistrationData{
		StellarAccount:   sep24Claims.SEP10StellarAccount(),
		JWTToken:         token,
		ReCAPTCHASiteKey: h.ReCAPTCHASiteKey,
	}

	htmlTemplateName := "receiver_register.tmpl"
	if rw != nil {
		// If the user was previously registered successfully, load a different template.
		htmlTemplateName = "receiver_registered_successfully.tmpl"
		tmplData.Title = "Registration Complete 🎉"
		tmplData.Message = "Your Stellar wallet has been registered successfully!"
	}

	registerPage, err := htmlTpl.ExecuteHTMLTemplate(htmlTemplateName, tmplData)
	if err != nil {
		httperror.InternalError(ctx, "Cannot process the html template for request", err, nil).Render(w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(registerPage))
	if err != nil {
		httperror.InternalError(ctx, "Cannot write html content to response", err, nil).Render(w)
		return
	}
}
