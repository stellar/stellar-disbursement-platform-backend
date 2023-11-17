package httphandler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/stellar/go/support/http/httpdecode"
	"github.com/stellar/go/support/log"
	"github.com/stellar/go/support/render/httpjson"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/serve/httperror"
	"github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/internal/provisioning"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/internal/validators"
	"github.com/stellar/stellar-disbursement-platform-backend/stellar-multitenant/pkg/tenant"
)

type TenantsHandler struct {
	Manager             *tenant.Manager
	ProvisioningManager *provisioning.Manager
	NetworkType         utils.NetworkType
}

func (t TenantsHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tnts, err := t.Manager.GetAllTenants(ctx)
	if err != nil {
		httperror.InternalError(ctx, "Cannot get tenants", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnts, httpjson.JSON)
}

func (t TenantsHandler) GetByIDOrName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	arg := chi.URLParam(r, "arg")

	tnt, err := t.Manager.GetTenantByIDOrName(ctx, arg)
	if err != nil {
		if errors.Is(tenant.ErrTenantDoesNotExist, err) {
			errorMsg := fmt.Sprintf("tenant %s does not exist", arg)
			httperror.NotFound(errorMsg, err, nil).Render(w)
			return
		}
		httperror.InternalError(ctx, "Cannot get tenant by ID or name", err, nil).Render(w)
		return
	}

	httpjson.RenderStatus(w, http.StatusOK, tnt, httpjson.JSON)
}

func (h TenantsHandler) PostTenants(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var reqBody *validators.TenantRequest
	if err := httpdecode.DecodeJSON(req, &reqBody); err != nil {
		log.Ctx(ctx).Errorf("decoding request body: %v", err)
		httperror.BadRequest("", err, nil).Render(rw)
		return
	}

	validator := validators.NewTenantValidator()
	reqBody = validator.ValidateCreateTenantRequest(reqBody)
	if validator.HasErrors() {
		httperror.BadRequest("invalid request body", nil, validator.Errors).Render(rw)
		return
	}

	tnt, err := h.ProvisioningManager.ProvisionNewTenant(
		ctx, reqBody.Name, reqBody.OwnerFirstName,
		reqBody.OwnerLastName, reqBody.OwnerEmail, reqBody.OrganizationName,
		reqBody.SDPUIBaseURL, string(h.NetworkType),
	)
	if err != nil {
		httperror.InternalError(ctx, "Could not provision a new tenant", err, nil).Render(rw)
		return
	}

	tnt, err = h.Manager.UpdateTenantConfig(ctx, &tenant.TenantUpdate{
		ID:                    tnt.ID,
		EmailSenderType:       &reqBody.EmailSenderType,
		SMSSenderType:         &reqBody.SMSSenderType,
		SEP10SigningPublicKey: &reqBody.SEP10SigningPublicKey,
		DistributionPublicKey: &reqBody.DistributionPublicKey,
		EnableMFA:             &reqBody.EnableMFA,
		EnableReCAPTCHA:       &reqBody.EnableReCAPTCHA,
		CORSAllowedOrigins:    reqBody.CORSAllowedOrigins,
		BaseURL:               &reqBody.BaseURL,
	})
	if err != nil {
		httperror.InternalError(ctx, "Could not update tenant config", err, nil).Render(rw)
		return
	}

	httpjson.RenderStatus(rw, http.StatusCreated, tnt, httpjson.JSON)
}
