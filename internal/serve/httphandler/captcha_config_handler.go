package httphandler

import (
	"net/http"

	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/sdpcontext"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type CAPTCHAConfigHandler struct {
	TenantManager     tenant.ManagerInterface
	Models            *data.Models
	CAPTCHAType       validators.CAPTCHAType
	ReCAPTCHADisabled bool
}

type CAPTCHAConfigResponse struct {
	CAPTCHAType     string `json:"captcha_type"`
	CAPTCHADisabled bool   `json:"captcha_disabled"`
}

func (h CAPTCHAConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	orgName := r.URL.Query().Get("organization_name")
	if orgName == "" {
		httperror.BadRequest("organization_name query parameter is required", nil, nil).Render(w)
		return
	}

	tnt, err := h.TenantManager.GetTenantByName(ctx, orgName)
	if err != nil {
		httperror.NotFound("organization not found", err, nil).Render(w)
		return
	}

	ctx = sdpcontext.SetTenantInContext(ctx, tnt)

	captchaDisabled := IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	})

	httpjson.Render(w, CAPTCHAConfigResponse{
		CAPTCHAType:     h.CAPTCHAType.String(),
		CAPTCHADisabled: captchaDisabled,
	}, httpjson.JSON)
}
