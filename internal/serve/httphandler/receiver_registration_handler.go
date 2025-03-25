package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"

	"github.com/stellar/stellar-disbursement-platform-backend/internal/anchorplatform"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/data"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type ReceiverRegistrationHandler struct {
	Models              *data.Models
	ReceiverWalletModel *data.ReceiverWalletModel
	ReCAPTCHADisabled   bool
	ReCAPTCHASiteKey    string
}

type ReceiverRegistrationResponse struct {
	PrivacyPolicyLink    string `json:"privacy_policy_link"`
	OrganizationName     string `json:"organization_name"`
	OrganizationLogo     string `json:"organization_logo"`
	TruncatedContactInfo string `json:"truncated_contact_info,omitempty"`
	IsRegistered         bool   `json:"is_registered"`
	IsRecaptchaDisabled  bool   `json:"is_recaptcha_disabled"`
	ReCAPTCHASiteKey     string `json:"recaptcha_site_key"`
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

	err := sep24Claims.Valid()
	if err != nil {
		err = fmt.Errorf("SEP-24 claims are invalid: %w", err)
		log.Ctx(ctx).Error(err)
		httperror.Unauthorized("", err, nil).Render(w)
		return
	}

	organization, err := h.Models.Organizations.Get(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get organization", err, nil).Render(w)
		return
	}

	privacyPolicyLink := ""
	if organization.PrivacyPolicyLink != nil {
		privacyPolicyLink = *organization.PrivacyPolicyLink
	}

	currentTenant, err := tenant.GetTenantFromContext(ctx)
	if err != nil || currentTenant == nil {
		httperror.InternalError(ctx, "Cannot retrieve the tenant from the context", err, nil).Render(w)
		return
	}

	logoURL, err := getLogoURL(currentTenant.BaseURL)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get logo URL", err, nil).Render(w)
		return
	}

	response := ReceiverRegistrationResponse{
		PrivacyPolicyLink:   privacyPolicyLink,
		OrganizationName:    organization.Name,
		OrganizationLogo:    logoURL,
		IsRecaptchaDisabled: h.ReCAPTCHADisabled,
		ReCAPTCHASiteKey:    h.ReCAPTCHASiteKey,
	}

	rw, err := h.ReceiverWalletModel.GetByStellarAccountAndMemo(ctx, sep24Claims.SEP10StellarAccount(), sep24Claims.SEP10StellarMemo(), sep24Claims.ClientDomain())
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		httperror.InternalError(ctx, "Cannot register receiver wallet", err, nil).Render(w)
		return
	}

	if rw != nil {
		response.IsRegistered = true
		response.TruncatedContactInfo = utils.TruncateString(rw.OTPConfirmedWith, 3)
	}

	httpjson.RenderStatus(w, http.StatusOK, response, httpjson.JSON)
}
