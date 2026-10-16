package httphandler

import (
	"net/http"

	"github.com/stellar/go-stellar-sdk/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/validators"
)

type AppConfigHandler struct {
	Models            *data.Models
	CAPTCHAType       validators.CAPTCHAType
	ReCAPTCHASiteKey  string
	ReCAPTCHADisabled bool
}

type AppConfigResponse struct {
	CAPTCHAType      string `json:"captcha_type"`
	CAPTCHASiteKey   string `json:"captcha_site_key"`
	CAPTCHADisabled  bool   `json:"captcha_disabled"`
	ReportingEnabled bool   `json:"reporting_enabled"`
}

func (h AppConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	captchaDisabled := IsCAPTCHADisabled(ctx, CAPTCHAConfig{
		Models:            h.Models,
		ReCAPTCHADisabled: h.ReCAPTCHADisabled,
	})

	var reportingEnabled bool
	if org, err := h.Models.Organizations.Get(ctx); err == nil && org.ReportingEnabled != nil {
		reportingEnabled = *org.ReportingEnabled
	}

	httpjson.Render(w, AppConfigResponse{
		CAPTCHAType:      h.CAPTCHAType.String(),
		CAPTCHASiteKey:   h.ReCAPTCHASiteKey,
		CAPTCHADisabled:  captchaDisabled,
		ReportingEnabled: reportingEnabled,
	}, httpjson.JSON)
}
